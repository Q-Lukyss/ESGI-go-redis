// Package ws est l'adaptateur temps réel : diffuse core.GoRedis.Changes()
// et les compteurs atomiques aux clients connectés, chacun filtré sur la
// fenêtre qu'il a déclarée. Miroir du pont wasmbridge côté backend WASM —
// même logique de fenêtres/coalescing, seul le "tuyau de sortie" diffère
// (WebSocket ici, postMessage là-bas).
package ws

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/samber/lo"

	"ESGI-go-redis/core"
)

const (
	sendBufferSize   = 64
	coalesceInterval = 250 * time.Millisecond
	statsInterval    = 250 * time.Millisecond
)

var upgrader = websocket.Upgrader{
	// Démo locale, pas de front tiers à filtrer par origine.
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	conn *websocket.Conn
	send chan []byte

	mu  sync.RWMutex
	win clientMessage
}

func (c *client) window() clientMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.win
}

func (c *client) setWindow(w clientMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.win = w
}

// sendNonBlocking pousse un message au client sans jamais bloquer le hub :
// un client lent qui n'a pas vidé son buffer perd le message plutôt que de
// ralentir la diffusion aux autres.
func (c *client) sendNonBlocking(msg []byte) {
	select {
	case c.send <- msg:
	default:
	}
}

// Hub diffuse les changements du moteur (patchs coalescés + compteurs) à
// tous les clients WS connectés.
type Hub struct {
	engine *core.GoRedis

	mu      sync.Mutex
	clients map[*client]struct{}
}

func NewHub(engine *core.GoRedis) *Hub {
	return &Hub{engine: engine, clients: make(map[*client]struct{})}
}

// Handler upgrade la connexion HTTP en WebSocket et enregistre le client.
func (h *Hub) Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, sendBufferSize)}

	h.register(c)
	go h.writePump(c)
	h.readPump(c) // bloque jusqu'à déconnexion
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
}

// readPump lit les messages de souscription du client jusqu'à déconnexion.
func (h *Hub) readPump(c *client) {
	defer func() {
		h.unregister(c)
		c.conn.Close()
	}()
	for {
		var msg clientMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			return
		}
		c.setWindow(msg)
	}
}

// writePump est l'unique goroutine qui écrit sur la connexion : gorilla/
// websocket ne supporte pas les écritures concurrentes sur un même conn.
func (h *Hub) writePump(c *client) {
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// Run draine core.GoRedis.Changes(), coalesce sur coalesceInterval et
// diffuse aux clients concernés ; un ticker séparé diffuse les compteurs.
// Bloque jusqu'à stop fermé — à lancer dans sa propre goroutine.
func (h *Hub) Run(stop <-chan struct{}) {
	changesTicker := time.NewTicker(coalesceInterval)
	statsTicker := time.NewTicker(statsInterval)
	defer changesTicker.Stop()
	defer statsTicker.Stop()

	pending := make(map[string]core.ChangeEvent)

	for {
		select {
		case ev, ok := <-h.engine.Changes():
			if !ok {
				return
			}
			pending[ev.Key] = ev // dernier évènement par clé gagne dans la fenêtre de coalescing

		case <-changesTicker.C:
			if len(pending) > 0 {
				h.broadcastPatch(pending)
				pending = make(map[string]core.ChangeEvent)
			}

		case <-statsTicker.C:
			h.broadcastStats()

		case <-stop:
			return
		}
	}
}

// broadcastPatch ne pousse que les mises à jour de valeur sur des clés déjà
// existantes (patchables en place), et seulement aux clients dont la
// fenêtre déclarée les couvre. Les insertions/suppressions (structurelles)
// ne sont jamais splicées en direct — elles se reflètent via
// broadcastStats, à charge du client d'en faire un "N nouveaux, afficher".
func (h *Hub) broadcastPatch(pending map[string]core.ChangeEvent) {
	entries := lo.FilterMap(lo.Values(pending), func(ev core.ChangeEvent, _ int) (core.BrowseEntry, bool) {
		if ev.Type != core.CmdSet || ev.IsNew {
			return core.BrowseEntry{}, false
		}
		return core.BrowseEntry{Key: ev.Key, Value: ev.Value, Timestamp: ev.Timestamp}, true
	})
	if len(entries) == 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		matched := filterInWindow(entries, c.window())
		if len(matched) == 0 {
			continue
		}
		msg, err := json.Marshal(serverMessage{Type: "patch", Entries: matched})
		if err != nil {
			continue
		}
		c.sendNonBlocking(msg)
	}
}

func (h *Hub) broadcastStats() {
	msg, err := json.Marshal(serverMessage{
		Type:        "stats",
		StateCount:  h.engine.StateCount(),
		BufferCount: h.engine.BufferCount(),
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.sendNonBlocking(msg)
	}
}

// filterInWindow ne garde que les entrées dont la position (clé ou
// timestamp, selon le mode déclaré par le client) tombe dans sa fenêtre.
// Un client qui n'a encore rien déclaré (mode vide) ne reçoit aucun patch.
func filterInWindow(entries []core.BrowseEntry, win clientMessage) []core.BrowseEntry {
	return lo.Filter(entries, func(e core.BrowseEntry, _ int) bool {
		switch win.Mode {
		case core.BrowseByKey:
			return inRangeString(e.Key, win.WindowStart, win.WindowEnd)
		case core.BrowseByTime:
			return inRangeInt64(e.Timestamp, win.WindowStart, win.WindowEnd)
		default:
			return false
		}
	})
}

func inRangeString(v, start, end string) bool {
	if start != "" && v < start {
		return false
	}
	if end != "" && v > end {
		return false
	}
	return true
}

func inRangeInt64(v int64, start, end string) bool {
	if start != "" {
		if s, err := strconv.ParseInt(start, 10, 64); err == nil && v < s {
			return false
		}
	}
	if end != "" {
		if e, err := strconv.ParseInt(end, 10, 64); err == nil && v > e {
			return false
		}
	}
	return true
}

//go:build js && wasm

// Package wasmbridge est l'adaptateur temps réel du backend WASM : reçoit
// des commandes du thread principal via postMessage et diffuse
// core.GoRedis.Changes() + les compteurs atomiques en retour. Miroir
// d'infrastructure/ws — même logique de fenêtre/coalescing, seul le
// "tuyau" diffère (postMessage ici, WebSocket côté serveur).
package wasmbridge

import (
	"encoding/json"
	"syscall/js"
	"time"

	"github.com/samber/lo"

	"ESGI-go-redis/core"
	"ESGI-go-redis/seed"
)

const (
	coalesceInterval = 250 * time.Millisecond
	statsInterval    = 250 * time.Millisecond

	defaultBrowseLimit = 100
	maxBrowseLimit     = 1000

	defaultSeedCount  = 100_000
	defaultSeedSpread = 720 * time.Hour
)

// Bridge branche un core.GoRedis sur postMessage. Un Worker = un onglet =
// un "client" : pas besoin d'un registre de connexions comme ws.Hub.
type Bridge struct {
	engine *core.GoRedis
	win    window
}

func New(engine *core.GoRedis) *Bridge {
	return &Bridge{engine: engine}
}

// Start enregistre le handler self.onmessage et lance la diffusion en
// arrière-plan (patchs coalescés + stats). Bloque jusqu'à stop fermé (à
// lancer dans sa propre goroutine, comme ws.Hub.Run).
func (b *Bridge) Start(stop <-chan struct{}) {
	js.Global().Set("onmessage", js.FuncOf(b.onMessage))
	postJSON(outboundMessage{Type: "ready"})
	b.run(stop)
}

func (b *Bridge) onMessage(this js.Value, args []js.Value) any {
	raw := js.Global().Get("JSON").Call("stringify", args[0].Get("data")).String()
	var msg inboundMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil
	}
	b.handle(msg)
	return nil
}

func (b *Bridge) handle(msg inboundMessage) {
	switch msg.Type {
	case "set":
		b.engine.Set(msg.Key, msg.Value)
		// Ack optionnel : seulement si le client a fourni un requestId (le
		// pont reste compatible avec un set "fire-and-forget"). Nécessaire
		// pour que le benchmark côté front mesure un vrai aller-retour,
		// comparable à la latence REST du backend serveur.
		if msg.RequestID != "" {
			postJSON(outboundMessage{Type: "response", RequestID: msg.RequestID})
		}

	case "get":
		value, err := b.engine.Get(msg.Key)
		if err != nil {
			b.reject(msg.RequestID, err)
			return
		}
		postJSON(outboundMessage{Type: "response", RequestID: msg.RequestID, Value: value})

	case "delete":
		if err := b.engine.Delete(msg.Key); err != nil {
			b.reject(msg.RequestID, err)
			return
		}
		postJSON(outboundMessage{Type: "response", RequestID: msg.RequestID})

	case "browse":
		mode := msg.Mode
		if mode == "" {
			mode = core.BrowseByKey
		}
		limit := msg.Limit
		if limit <= 0 {
			limit = defaultBrowseLimit
		}
		if limit > maxBrowseLimit {
			limit = maxBrowseLimit
		}
		entries, nextCursor, hasMore, err := b.engine.Browse(mode, msg.Cursor, limit)
		if err != nil {
			b.reject(msg.RequestID, err)
			return
		}
		postJSON(outboundMessage{Type: "response", RequestID: msg.RequestID, Entries: entries, NextCursor: nextCursor, HasMore: hasMore})

	case "stats":
		postJSON(outboundMessage{Type: "response", RequestID: msg.RequestID, StateCount: b.engine.StateCount(), BufferCount: b.engine.BufferCount()})

	case "query":
		matches, err := b.engine.GetWhere(core.FilterOp(msg.Op), msg.FilterValue)
		if err != nil {
			b.reject(msg.RequestID, err)
			return
		}
		postJSON(outboundMessage{Type: "response", RequestID: msg.RequestID, Matches: matches})

	case "subscribe":
		b.win = window{mode: msg.Mode, windowStart: msg.WindowStart, windowEnd: msg.WindowEnd}

	case "seed":
		// Un process natif ne peut pas écrire dans l'OPFS d'un navigateur :
		// le peuplement de démo doit se faire ici, au runtime. En goroutine
		// pour ne pas bloquer onMessage (peut prendre plusieurs secondes).
		go b.runSeed(msg.RequestID, msg.Count, msg.SpreadHours)
	}
}

func (b *Bridge) reject(requestID string, err error) {
	postJSON(outboundMessage{Type: "response", RequestID: requestID, Error: err.Error()})
}

func (b *Bridge) runSeed(requestID string, count, spreadHours int) {
	if count <= 0 {
		count = defaultSeedCount
	}
	spread := defaultSeedSpread
	if spreadHours > 0 {
		spread = time.Duration(spreadHours) * time.Hour
	}

	entries := seed.Generate(count, spread, time.Now(), 42)
	for _, e := range entries {
		b.engine.SetAt(e.Key, e.Value, e.Timestamp)
	}
	_ = b.engine.Snapshot()

	postJSON(outboundMessage{Type: "seedDone", RequestID: requestID, StateCount: b.engine.StateCount()})
}

// run draine core.GoRedis.Changes(), coalesce et diffuse — même logique
// que ws.Hub.Run, sans le registre de clients (un seul destinataire ici).
func (b *Bridge) run(stop <-chan struct{}) {
	changesTicker := time.NewTicker(coalesceInterval)
	statsTicker := time.NewTicker(statsInterval)
	defer changesTicker.Stop()
	defer statsTicker.Stop()

	pending := make(map[string]core.ChangeEvent)

	for {
		select {
		case ev, ok := <-b.engine.Changes():
			if !ok {
				return
			}
			pending[ev.Key] = ev

		case <-changesTicker.C:
			if len(pending) > 0 {
				b.broadcastPatch(pending)
				pending = make(map[string]core.ChangeEvent)
			}

		case <-statsTicker.C:
			postJSON(outboundMessage{Type: "stats", StateCount: b.engine.StateCount(), BufferCount: b.engine.BufferCount()})

		case <-stop:
			return
		}
	}
}

func (b *Bridge) broadcastPatch(pending map[string]core.ChangeEvent) {
	entries := lo.FilterMap(lo.Values(pending), func(ev core.ChangeEvent, _ int) (core.BrowseEntry, bool) {
		if ev.Type != core.CmdSet || ev.IsNew {
			return core.BrowseEntry{}, false
		}
		return core.BrowseEntry{Key: ev.Key, Value: ev.Value, Timestamp: ev.Timestamp}, true
	})
	matched := filterInWindow(entries, b.win)
	if len(matched) == 0 {
		return
	}
	postJSON(outboundMessage{Type: "patch", Entries: matched})
}

// postJSON sérialise v en JSON puis le repasse par JSON.parse côté JS avant
// postMessage : plus simple et moins sujet aux erreurs que de reconstruire
// un js.Value champ par champ pour chaque type de message.
func postJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	jsObj := js.Global().Get("JSON").Call("parse", string(data))
	js.Global().Call("postMessage", jsObj)
}

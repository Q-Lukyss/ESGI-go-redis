// Package http est l'adaptateur REST : CRUD simple + parcours paginé
// (scroll infini) au-dessus d'un core.GoRedis. Ne connaît rien de la
// syntaxe texte du REPL ni du hub WS — juste le moteur.
package http

import (
	"net/http"

	"ESGI-go-redis/core"
)

// NewRouter construit le multiplexeur REST. wsHandler est branché tel quel
// sur /ws (fourni par infrastructure/ws, pour ne pas faire dépendre ce
// package du hub WebSocket).
func NewRouter(engine *core.GoRedis, wsHandler http.HandlerFunc) *http.ServeMux {
	h := &handlers{engine: engine}
	mux := http.NewServeMux()

	mux.HandleFunc("PUT /keys/{key}", h.setKey)
	mux.HandleFunc("GET /keys/{key}", h.getKey)
	mux.HandleFunc("DELETE /keys/{key}", h.deleteKey)
	mux.HandleFunc("GET /query", h.query)
	mux.HandleFunc("GET /browse", h.browse)
	mux.HandleFunc("GET /stats", h.stats)
	if wsHandler != nil {
		mux.HandleFunc("GET /ws", wsHandler)
	}

	return mux
}

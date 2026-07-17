package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"ESGI-go-redis/core"
)

// maxBrowseLimit borne la taille de page demandable par le client : sans
// ça, un ?limit=1000000000 forcerait le moteur à matérialiser une page
// énorme d'un coup — exactement ce que la pagination par curseur est censée
// éviter.
const (
	defaultBrowseLimit = 100
	maxBrowseLimit     = 1000
)

type handlers struct {
	engine *core.GoRedis
}

type setRequest struct {
	Value string `json:"value"`
}

func (h *handlers) setKey(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var body setRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.engine.Set(key, body.Value)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getKey(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	value, err := h.engine.Get(key)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

func (h *handlers) deleteKey(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if err := h.engine.Delete(key); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) query(w http.ResponseWriter, r *http.Request) {
	op := core.FilterOp(r.URL.Query().Get("op"))
	value := r.URL.Query().Get("value")
	matches, err := h.engine.GetWhere(op, value)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, matches)
}

type browseResponse struct {
	Entries    []core.BrowseEntry `json:"entries"`
	NextCursor string             `json:"nextCursor"`
	HasMore    bool               `json:"hasMore"`
}

func (h *handlers) browse(w http.ResponseWriter, r *http.Request) {
	mode := core.BrowseMode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = core.BrowseByKey
	}
	cursor := r.URL.Query().Get("cursor")

	limit := defaultBrowseLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxBrowseLimit {
		limit = maxBrowseLimit
	}

	entries, nextCursor, hasMore, err := h.engine.Browse(mode, cursor, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, browseResponse{Entries: entries, NextCursor: nextCursor, HasMore: hasMore})
}

type statsResponse struct {
	StateCount  int64 `json:"stateCount"`
	BufferCount int64 `json:"bufferCount"`
}

func (h *handlers) stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statsResponse{
		StateCount:  h.engine.StateCount(),
		BufferCount: h.engine.BufferCount(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

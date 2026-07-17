//go:build js && wasm

package wasmbridge

import "ESGI-go-redis/core"

// inboundMessage est un message reçu du thread principal via postMessage.
// Union taggée par Type (set/get/delete/browse/stats/query/subscribe/seed) ;
// mêmes noms de champs que le protocole WS (infrastructure/ws) pour que
// wasmClient.ts et serverClient.ts partagent un maximum de code côté front.
type inboundMessage struct {
	Type        string          `json:"type"`
	RequestID   string          `json:"requestId,omitempty"`
	Key         string          `json:"key,omitempty"`
	Value       string          `json:"value,omitempty"`
	Mode        core.BrowseMode `json:"mode,omitempty"`
	Cursor      string          `json:"cursor,omitempty"`
	Limit       int             `json:"limit,omitempty"`
	WindowStart string          `json:"windowStart,omitempty"`
	WindowEnd   string          `json:"windowEnd,omitempty"`
	Op          string          `json:"op,omitempty"`
	FilterValue string          `json:"filterValue,omitempty"`
	Count       int             `json:"count,omitempty"`
	SpreadHours int             `json:"spreadHours,omitempty"`
}

// outboundMessage est un message envoyé au thread principal.
//   - "ready"    : le moteur a fini son Restore(), prêt à recevoir des requêtes.
//   - "response" : réponse corrélée par RequestID à get/delete/browse/stats/query.
//   - "patch"    : mises à jour de valeur dans la fenêtre déclarée (cf. subscribe).
//   - "stats"    : compteurs globaux, diffusés en continu.
//   - "seedDone" : fin du peuplement déclenché par "seed".
type outboundMessage struct {
	Type        string             `json:"type"`
	RequestID   string             `json:"requestId,omitempty"`
	Value       string             `json:"value,omitempty"`
	Entries     []core.BrowseEntry `json:"entries,omitempty"`
	NextCursor  string             `json:"nextCursor,omitempty"`
	HasMore     bool               `json:"hasMore,omitempty"`
	StateCount  int64              `json:"stateCount,omitempty"`
	BufferCount int64              `json:"bufferCount,omitempty"`
	Matches     []core.Match       `json:"matches,omitempty"`
	Error       string             `json:"error,omitempty"`
}

// window est la fenêtre visible déclarée par le seul "client" possible ici
// (un onglet = un Worker = une connexion) — équivalent de clientMessage
// dans infrastructure/ws, mais pas besoin d'un registre de plusieurs
// clients.
type window struct {
	mode        core.BrowseMode
	windowStart string
	windowEnd   string
}

package ws

import "ESGI-go-redis/core"

// clientMessage est un message entrant (client -> serveur) : déclare la
// fenêtre actuellement visible côté UI, pour que le hub ne pousse que les
// changements pertinents à ce client plutôt que de broadcaster tout le
// store à chaque écriture.
type clientMessage struct {
	Mode core.BrowseMode `json:"mode"`
	// WindowStart/WindowEnd bornent la fenêtre déclarée : une clé en mode
	// "key" (comparée lexicographiquement), un timestamp unix-nano en
	// chaîne en mode "time" (comparé numériquement). Borne vide = non
	// contraignante de ce côté.
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`
}

// serverMessage est un message sortant, discriminé par Type :
//   - "patch" : mises à jour de valeur sur des clés existantes, dans la
//     fenêtre du client — à appliquer en place, sans bouger la ligne.
//   - "stats" : compteurs globaux, diffusés à tous quel que soit leur
//     fenêtre (c'est aussi via ce canal qu'un client détecte des
//     insertions/suppressions ailleurs dans le store : le compteur bouge
//     sans qu'aucune ligne chargée n'ait changé).
type serverMessage struct {
	Type        string             `json:"type"`
	Entries     []core.BrowseEntry `json:"entries,omitempty"`
	StateCount  int64              `json:"stateCount,omitempty"`
	BufferCount int64              `json:"bufferCount,omitempty"`
}

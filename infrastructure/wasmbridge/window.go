//go:build js && wasm

package wasmbridge

import (
	"strconv"

	"github.com/samber/lo"

	"ESGI-go-redis/core"
)

// filterInWindow ne garde que les entrées dont la position (clé ou
// timestamp, selon le mode déclaré) tombe dans la fenêtre visible déclarée
// par le client (cf. case "subscribe" dans bridge.go) — évite de repousser
// tout le store à chaque patch.
func filterInWindow(entries []core.BrowseEntry, win window) []core.BrowseEntry {
	return lo.Filter(entries, func(e core.BrowseEntry, _ int) bool {
		switch win.mode {
		case core.BrowseByKey:
			return inRangeString(e.Key, win.windowStart, win.windowEnd)
		case core.BrowseByTime:
			return inRangeInt64(e.Timestamp, win.windowStart, win.windowEnd)
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

package engine

import (
	"testing"
	"time"
)

// newTestEngine crée un moteur avec un FileStorage sur un dossier temporaire,
// prêt pour les tests. Les intervalles de flush/snapshot sont volontairement
// longs : les tests déclenchent Flush()/Snapshot() explicitement.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	storage := NewFileStorage(dir+"/aof.jsonl", dir+"/snapshot.json")
	return NewEngine(storage, time.Hour, time.Hour)
}

func TestEngineSetGet(t *testing.T) {
	e := newTestEngine(t)
	e.Set("name", "matt")

	value, err := e.Get("name")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if value != "matt" {
		t.Fatalf("got %q, want %q", value, "matt")
	}
}

func TestEngineGetMissingKey(t *testing.T) {
	e := newTestEngine(t)
	if _, err := e.Get("missing"); err == nil {
		t.Fatal("attendu une erreur pour une clé absente")
	}
}

func TestEngineDeleteThenGet(t *testing.T) {
	e := newTestEngine(t)
	e.Set("name", "matt")

	if err := e.Delete("name"); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if _, err := e.Get("name"); err == nil {
		t.Fatal("attendu une erreur après suppression")
	}
}

func TestEngineDeleteMissingKey(t *testing.T) {
	e := newTestEngine(t)
	if err := e.Delete("missing"); err == nil {
		t.Fatal("attendu une erreur pour la suppression d'une clé absente")
	}
}

func TestEngineOverwrite(t *testing.T) {
	e := newTestEngine(t)
	e.Set("name", "matt")
	e.Set("name", "alice")

	value, err := e.Get("name")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if value != "alice" {
		t.Fatalf("got %q, want %q", value, "alice")
	}
}

func TestEngineExecuteString(t *testing.T) {
	e := newTestEngine(t)

	if _, err := e.ExecuteString(`SET name "matt"`); err != nil {
		t.Fatalf("erreur inattendue au SET: %v", err)
	}

	result, err := e.ExecuteString("GET name")
	if err != nil {
		t.Fatalf("erreur inattendue au GET: %v", err)
	}
	if result != "matt" {
		t.Fatalf("got %v, want %q", result, "matt")
	}

	if _, err := e.ExecuteString("DELETE name"); err != nil {
		t.Fatalf("erreur inattendue au DELETE: %v", err)
	}
	if _, err := e.ExecuteString("GET name"); err == nil {
		t.Fatal("attendu une erreur après suppression")
	}
}

func TestEngineExecuteStringPropagatesParseError(t *testing.T) {
	e := newTestEngine(t)
	if _, err := e.ExecuteString("PING"); err == nil {
		t.Fatal("attendu une erreur de parsing propagée")
	}
}

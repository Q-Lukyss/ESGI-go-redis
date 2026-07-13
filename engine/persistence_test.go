package engine

import (
	"testing"
	"time"
)

// TestRestoreAfterRestart est le test en or de la Phase 2 : on remplit un
// moteur, on le "tue" (on ne garde que ce qui est sur disque), on en relance
// un nouveau sur le même dossier et on vérifie que l'état est identique.
func TestRestoreAfterRestart(t *testing.T) {
	dir := t.TempDir()
	aofPath := dir + "/aof.jsonl"
	snapshotPath := dir + "/snapshot.json"

	first := NewEngine(NewFileStorage(aofPath, snapshotPath), time.Hour, time.Hour)
	first.Set("name", "matt")
	first.Set("age", "30")
	first.Delete("age")
	if err := first.Flush(); err != nil {
		t.Fatalf("erreur au flush: %v", err)
	}

	second := NewEngine(NewFileStorage(aofPath, snapshotPath), time.Hour, time.Hour)
	if err := second.Restore(); err != nil {
		t.Fatalf("erreur au restore: %v", err)
	}

	value, err := second.Get("name")
	if err != nil {
		t.Fatalf("clé attendue absente après restore: %v", err)
	}
	if value != "matt" {
		t.Fatalf("got %q, want %q", value, "matt")
	}
	if _, err := second.Get("age"); err == nil {
		t.Fatal("clé supprimée réapparue après restore")
	}
}

func TestRestoreAppliesAOFOverSnapshot(t *testing.T) {
	dir := t.TempDir()
	aofPath := dir + "/aof.jsonl"
	snapshotPath := dir + "/snapshot.json"

	first := NewEngine(NewFileStorage(aofPath, snapshotPath), time.Hour, time.Hour)
	first.Set("name", "matt")
	if err := first.Snapshot(); err != nil { // photo complète + vide l'AOF (compaction)
		t.Fatalf("erreur au snapshot: %v", err)
	}
	first.Set("name", "alice") // écriture après la photo : seulement dans l'AOF
	if err := first.Flush(); err != nil {
		t.Fatalf("erreur au flush: %v", err)
	}

	second := NewEngine(NewFileStorage(aofPath, snapshotPath), time.Hour, time.Hour)
	if err := second.Restore(); err != nil {
		t.Fatalf("erreur au restore: %v", err)
	}

	value, err := second.Get("name")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if value != "alice" {
		t.Fatalf("got %q, want %q (l'AOF doit s'appliquer par-dessus le snapshot)", value, "alice")
	}
}

func TestSnapshotClearsAOF(t *testing.T) {
	dir := t.TempDir()
	storage := NewFileStorage(dir+"/aof.jsonl", dir+"/snapshot.json")
	e := NewEngine(storage, time.Hour, time.Hour)
	e.Set("name", "matt")

	if err := e.Snapshot(); err != nil {
		t.Fatalf("erreur au snapshot: %v", err)
	}

	ops, err := storage.ReadAOF()
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("AOF non vidée après snapshot: %+v", ops)
	}
}

func TestRestoreRebuildsIndexes(t *testing.T) {
	dir := t.TempDir()
	aofPath := dir + "/aof.jsonl"
	snapshotPath := dir + "/snapshot.json"

	first := NewEngine(NewFileStorage(aofPath, snapshotPath), time.Hour, time.Hour)
	first.Set("alice", "30")
	first.Set("bob", "30")
	if err := first.Snapshot(); err != nil {
		t.Fatalf("erreur au snapshot: %v", err)
	}

	second := NewEngine(NewFileStorage(aofPath, snapshotPath), time.Hour, time.Hour)
	if err := second.Restore(); err != nil {
		t.Fatalf("erreur au restore: %v", err)
	}

	matches, err := second.GetWhere(OpEquals, "30")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("index equals non reconstruit après restore: %+v", matches)
	}
}

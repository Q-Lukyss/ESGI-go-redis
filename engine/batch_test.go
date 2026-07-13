package engine

import "testing"

func TestExecuteBatch(t *testing.T) {
	e := newTestEngine(t)

	results := e.ExecuteBatch([]string{
		`SET name "matt"`,
		"GET name",
		"DELETE name",
		"GET name",
	})

	if len(results) != 4 {
		t.Fatalf("got %d résultats, want 4", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("SET a échoué: %v", results[0].Err)
	}
	if results[1].Err != nil || results[1].Value != "matt" {
		t.Fatalf("GET inattendu: value=%v err=%v", results[1].Value, results[1].Err)
	}
	if results[2].Err != nil {
		t.Fatalf("DELETE a échoué: %v", results[2].Err)
	}
	if results[3].Err == nil {
		t.Fatal("attendu une erreur pour GET après DELETE")
	}
}

func TestExecuteBatchPropagatesParseErrors(t *testing.T) {
	e := newTestEngine(t)

	results := e.ExecuteBatch([]string{
		`SET name "matt"`,
		"PING", // commande inconnue
	})

	if results[0].Err != nil {
		t.Fatalf("SET a échoué: %v", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatal("attendu une erreur de parsing pour la commande inconnue")
	}
}

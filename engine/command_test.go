package engine

import "testing"

func TestParseCommandSet(t *testing.T) {
	cmd, err := ParseCommand(`SET name "matt"`)
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	want := Command{Type: CmdSet, Key: "name", Value: "matt"}
	if cmd != want {
		t.Fatalf("got %+v, want %+v", cmd, want)
	}
}

func TestParseCommandSetWithSpacesInValue(t *testing.T) {
	cmd, err := ParseCommand(`SET msg "hello world"`)
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if cmd.Value != "hello world" {
		t.Fatalf("got value %q, want %q", cmd.Value, "hello world")
	}
}

func TestParseCommandSetMissingArgs(t *testing.T) {
	if _, err := ParseCommand("SET name"); err == nil {
		t.Fatal("attendu une erreur pour SET incomplet")
	}
}

func TestParseCommandGet(t *testing.T) {
	cmd, err := ParseCommand("GET name")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	want := Command{Type: CmdGet, Key: "name"}
	if cmd != want {
		t.Fatalf("got %+v, want %+v", cmd, want)
	}
}

func TestParseCommandGetMissingKey(t *testing.T) {
	if _, err := ParseCommand("GET"); err == nil {
		t.Fatal("attendu une erreur pour GET sans clé")
	}
}

func TestParseCommandDelete(t *testing.T) {
	cmd, err := ParseCommand("DELETE name")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	want := Command{Type: CmdDelete, Key: "name"}
	if cmd != want {
		t.Fatalf("got %+v, want %+v", cmd, want)
	}
}

func TestParseCommandUnknown(t *testing.T) {
	if _, err := ParseCommand("PING"); err == nil {
		t.Fatal("attendu une erreur pour une commande inconnue")
	}
}

func TestParseCommandCaseInsensitive(t *testing.T) {
	cmd, err := ParseCommand(`set name "matt"`)
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if cmd.Type != CmdSet {
		t.Fatalf("got type %v, want %v", cmd.Type, CmdSet)
	}
}

func TestParseCommandGetWhereEquals(t *testing.T) {
	cmd, err := ParseCommand(`GET WHERE value equals "matt"`)
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	want := Command{Type: CmdGetWhere, FilterOp: OpEquals, FilterValue: "matt"}
	if cmd != want {
		t.Fatalf("got %+v, want %+v", cmd, want)
	}
}

func TestParseCommandGetWhereRange(t *testing.T) {
	cmd, err := ParseCommand("GET WHERE value > 30")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	want := Command{Type: CmdGetWhere, FilterOp: OpGT, FilterValue: "30"}
	if cmd != want {
		t.Fatalf("got %+v, want %+v", cmd, want)
	}
}

func TestParseCommandGetWhereUnknownField(t *testing.T) {
	if _, err := ParseCommand(`GET WHERE age equals "30"`); err == nil {
		t.Fatal("attendu une erreur pour un champ inconnu")
	}
}

func TestParseCommandGetWhereUnknownOperator(t *testing.T) {
	if _, err := ParseCommand(`GET WHERE value near "30"`); err == nil {
		t.Fatal("attendu une erreur pour un opérateur inconnu")
	}
}

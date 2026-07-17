package core

// changesBufferSize borne le channel de notification : un consommateur lent
// (ou absent, ex. mode REPL pur) ne doit jamais bloquer une écriture.
const changesBufferSize = 4096

// ChangeEvent est la version diffusable d'une Operation, consommée par les
// transports (WS, wasmbridge) sans qu'ils aient besoin de connaître le
// moteur ou son verrou.
type ChangeEvent struct {
	Type      CommandType
	Key       string
	Value     string
	Timestamp int64
}

// Changes expose le flux de changements en lecture seule, pour qu'un
// transport (hub WS, pont WASM) puisse s'y brancher sans dépendre du reste
// du moteur.
func (e *GoRedis) Changes() <-chan ChangeEvent {
	return e.changes
}

// publishChangeLocked pousse un évènement sur le channel de notification,
// sans jamais bloquer : si personne n'écoute (buffer plein ou pas de
// consommateur), l'évènement est perdu plutôt que de ralentir l'écriture.
// Appelée sous verrou par recordOpLocked.
func (e *GoRedis) publishChangeLocked(op Operation) {
	event := ChangeEvent{Type: op.Type, Key: op.Key, Value: op.Value, Timestamp: op.Timestamp}
	select {
	case e.changes <- event:
	default:
	}
}

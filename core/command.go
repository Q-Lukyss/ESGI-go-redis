package core

// CommandType identifie la nature d'une commande parsée.
type CommandType string

const (
	CmdSet      CommandType = "SET"
	CmdGet      CommandType = "GET"
	CmdDelete   CommandType = "DELETE"
	CmdGetWhere CommandType = "GET_WHERE"
	CmdFlushAll CommandType = "FLUSHALL"
)

// FilterOp est l'opérateur utilisé par un GET WHERE.
type FilterOp string

const (
	OpEquals   FilterOp = "equals"
	OpContains FilterOp = "contains"
	OpGT       FilterOp = ">"
	OpGTE      FilterOp = ">="
	OpLT       FilterOp = "<"
	OpLTE      FilterOp = "<="
)

// Command est la représentation structurée d'une commande, transport-
// agnostique : REST, WS, wasmbridge et le REPL texte (infrastructure/repl)
// construisent tous un Command avant de le passer à GoRedis.Execute.
// Selon Type, seuls certains champs sont pertinents :
//   - CmdSet    : Key, Value, ExpireSeconds (optionnel, TTL via EX)
//   - CmdGet    : Key
//   - CmdDelete : Key
//   - CmdGetWhere : FilterOp, FilterValue
type Command struct {
	Type          CommandType
	Key           string
	Value         string
	ExpireSeconds int64 // TTL en secondes ; <= 0 = pas de TTL explicite (SET sans EX)
	FilterOp      FilterOp
	FilterValue   string
}

//go:build js && wasm

package wasmbridge

import "fmt"

// Bornes de sécurité, pas des constantes métier (cf. §7.2) : elles ne
// protègent pas un comportement business, seulement le moteur contre un
// message malformé ou hostile — ne jamais faire confiance à l'émetteur,
// même si le SDK/TS valide déjà côté client (§7.4).
const (
	maxKeyLength   = 1024
	maxValueLength = 1_000_000
)

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("la clé ne peut pas être vide")
	}
	if len(key) > maxKeyLength {
		return fmt.Errorf("clé trop longue (max %d octets)", maxKeyLength)
	}
	return nil
}

func validateValue(value string) error {
	if len(value) > maxValueLength {
		return fmt.Errorf("valeur trop longue (max %d octets)", maxValueLength)
	}
	return nil
}

func validateExpireSeconds(expireSeconds int64) error {
	if expireSeconds < 0 {
		return fmt.Errorf("expireSeconds doit être positif ou nul")
	}
	return nil
}

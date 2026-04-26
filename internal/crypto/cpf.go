package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// CPFHash returns a deterministic, non-reversible HMAC-SHA256 of the CPF.
// The same CPF + secret always produces the same hash, enabling DB lookups
// without storing the CPF in plaintext.
func CPFHash(cpf, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cpf))
	return hex.EncodeToString(mac.Sum(nil))
}

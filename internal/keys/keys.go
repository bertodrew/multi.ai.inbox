// Package keys creates and verifies API keys. Keys are random, shown once and
// stored as SHA-256 hashes (they carry 190 bits of entropy, so a slow hash adds nothing).
package keys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
)

const (
	LivePrefix = "mai_live_"
	alphabet   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	secretLen  = 32
)

// New returns the plaintext key, its display prefix and its hash.
func New() (plain, prefix string, hash []byte, err error) {
	var b strings.Builder
	b.WriteString(LivePrefix)
	max := big.NewInt(int64(len(alphabet)))
	for range secretLen {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", "", nil, fmt.Errorf("generate key: %w", err)
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	plain = b.String()
	return plain, plain[:len(LivePrefix)+6], Hash(plain), nil
}

func Hash(plain string) []byte {
	h := sha256.Sum256([]byte(plain))
	return h[:]
}

// WellFormed rejects obviously invalid keys before touching the database.
func WellFormed(plain string) bool {
	return strings.HasPrefix(plain, LivePrefix) && len(plain) == len(LivePrefix)+secretLen
}

// NewWebhookSecret returns a Standard Webhooks secret ("whsec_" + base64 of 32 bytes).
func NewWebhookSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(buf), nil
}

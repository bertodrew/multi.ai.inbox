package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// PKCE (RFC 7636) with the S256 method only: the plain method offers nothing
// over a public client, and every provider we use supports S256.
const (
	// VerifierBytes is 32 random bytes, encoded to 43 characters, inside the
	// 43 to 128 character range of RFC 7636 section 4.1.
	VerifierBytes = 32
)

// NewVerifier returns a code verifier for one authorization request.
func NewVerifier() (string, error) {
	buf := make([]byte, VerifierBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ChallengeS256 is BASE64URL(SHA256(ASCII(verifier))), the code_challenge sent
// to the provider (RFC 7636 section 4.2).
func ChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ValidVerifier checks the shape of a verifier we stored earlier.
func ValidVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '.', c == '_', c == '~':
		default:
			return false
		}
	}
	return true
}

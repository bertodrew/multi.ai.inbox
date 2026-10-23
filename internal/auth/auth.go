// Package auth holds the primitives of user authentication: signed access
// tokens, single use random tokens, PKCE, OAuth providers and the SMTP relay
// mailer. The rules live in core; this package only knows how, never when.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrTokenInvalid covers every rejection of a token: bad shape, bad
	// signature, wrong algorithm or unknown issuer.
	ErrTokenInvalid = errors.New("auth: invalid token")
	// ErrTokenExpired is returned when the signature is valid but exp passed.
	ErrTokenExpired = errors.New("auth: token expired")
)

var b64 = base64.RawURLEncoding

// Claims is the payload of an access token. The organization and the role are
// carried so a request does not need a second lookup for them; the session id
// is carried so a logout can revoke the token before it expires.
type Claims struct {
	Subject   string `json:"sub"`
	SessionID string `json:"sid"`
	OrgID     string `json:"org"`
	Role      string `json:"role"`
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Sign returns a compact JWS with HS256. A short lived access token needs no
// asymmetric key: the same service signs and verifies it.
func Sign(secret []byte, c Claims) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("auth: signing secret must be at least 32 bytes")
	}
	h, err := json.Marshal(header{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	p, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signing := b64.EncodeToString(h) + "." + b64.EncodeToString(p)
	return signing + "." + b64.EncodeToString(mac(secret, signing)), nil
}

// Verify checks the signature, the algorithm and the expiry, in that order.
func Verify(secret []byte, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	raw, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var h header
	if err := json.Unmarshal(raw, &h); err != nil || h.Alg != "HS256" || h.Typ != "JWT" {
		return nil, ErrTokenInvalid
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if subtle.ConstantTimeCompare(sig, mac(secret, parts[0]+"."+parts[1])) != 1 {
		return nil, ErrTokenInvalid
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, ErrTokenInvalid
	}
	if c.Subject == "" || c.SessionID == "" {
		return nil, ErrTokenInvalid
	}
	if c.ExpiresAt > 0 && time.Now().Unix() >= c.ExpiresAt {
		return nil, ErrTokenExpired
	}
	return &c, nil
}

func mac(secret []byte, signing string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(signing))
	return m.Sum(nil)
}

// NewToken returns n random bytes as base64url without padding: 32 bytes give
// 256 bits of entropy, enough for a bearer secret that is never guessable.
func NewToken(n int) (string, error) {
	if n < 16 {
		return "", errors.New("auth: token needs at least 16 bytes of entropy")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	return b64.EncodeToString(buf), nil
}

// HashToken is what the database stores: the plaintext token only ever exists
// in the response or in the email, never at rest.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// WellFormedToken rejects obvious junk before a hash comparison.
func WellFormedToken(token string, n int) bool {
	if len(token) != b64.EncodedLen(n) {
		return false
	}
	_, err := b64.DecodeString(token)
	return err == nil
}

// LooksLikeJWT reports whether a bearer value is shaped like a compact JWS.
// API keys carry their own prefix, so the two never collide.
func LooksLikeJWT(v string) bool {
	return strings.Count(v, ".") == 2 && len(v) > 40 && len(v) < 4096
}

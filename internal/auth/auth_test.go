package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func secret() []byte { return []byte("0123456789abcdef0123456789abcdef") }

func TestSignVerifyRoundTrip(t *testing.T) {
	c := Claims{Subject: "u1", SessionID: "s1", OrgID: "o1", Role: "owner", Issuer: "multi.ai.inbox",
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Minute).Unix()}
	token, err := Sign(secret(), c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(secret(), token)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "u1" || got.SessionID != "s1" || got.OrgID != "o1" || got.Role != "owner" {
		t.Fatalf("claims = %+v", got)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	token, err := Sign(secret(), Claims{Subject: "u1", SessionID: "s1", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	cases := map[string]string{
		"payload swapped": parts[0] + "." + b64.EncodeToString([]byte(`{"sub":"admin","sid":"s1","exp":9999999999}`)) + "." + parts[2],
		"signature cut":   strings.TrimSuffix(token, parts[2]) + "AAAA",
		"other secret":    "",
	}
	for name, tampered := range cases {
		if name == "other secret" {
			if _, err := Verify([]byte("ffffffffffffffffffffffffffffffff"), token); !errors.Is(err, ErrTokenInvalid) {
				t.Fatalf("%s: err = %v", name, err)
			}
			continue
		}
		if _, err := Verify(secret(), tampered); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if _, err := Verify(secret(), "not.a.jwt"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("junk: err = %v", err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	token, err := Sign(secret(), Claims{Subject: "u1", SessionID: "s1", ExpiresAt: time.Now().Add(-time.Second).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(secret(), token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignRequiresLongSecret(t *testing.T) {
	if _, err := Sign([]byte("short"), Claims{Subject: "u1"}); err == nil {
		t.Fatal("a short secret must be refused")
	}
}

func TestTokensAreUniqueAndHashed(t *testing.T) {
	a, err := NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two tokens must differ")
	}
	if !WellFormedToken(a, 32) {
		t.Fatalf("%q rejected as malformed", a)
	}
	if WellFormedToken(a, 16) {
		t.Fatal("a 32 byte token must not pass as 16 bytes")
	}
	if string(HashToken(a)) == a {
		t.Fatal("HashToken must not return the token")
	}
	if len(HashToken(a)) != 32 {
		t.Fatalf("hash length = %d", len(HashToken(a)))
	}
}

func TestLooksLikeJWT(t *testing.T) {
	token, err := Sign(secret(), Claims{Subject: "u1", SessionID: "s1", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if !LooksLikeJWT(token) {
		t.Fatal("a signed token must look like a JWT")
	}
	for _, v := range []string{"", "mai_live_abcdef", "a.b", strings.Repeat("x", 100)} {
		if LooksLikeJWT(v) {
			t.Fatalf("%q must not look like a JWT", v)
		}
	}
}

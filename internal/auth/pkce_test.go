package auth

import "testing"

// RFC 7636 appendix B: the example verifier and its S256 challenge.
func TestChallengeS256Vector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := ChallengeS256(verifier); got != want {
		t.Fatalf("ChallengeS256 = %q, want %q", got, want)
	}
}

func TestNewVerifierShape(t *testing.T) {
	seen := map[string]bool{}
	for range 20 {
		v, err := NewVerifier()
		if err != nil {
			t.Fatal(err)
		}
		if !ValidVerifier(v) {
			t.Fatalf("%q is not a valid verifier", v)
		}
		if seen[v] {
			t.Fatal("verifier repeated")
		}
		seen[v] = true
	}
}

func TestValidVerifierRejects(t *testing.T) {
	for _, v := range []string{"", "short", "with space in the verifier value that is long enough", string(make([]byte, 200))} {
		if ValidVerifier(v) {
			t.Fatalf("%q accepted", v)
		}
	}
}

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// rfcVerifier is the example verifier of RFC 7636 appendix B.
const rfcVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

// fakeProvider answers the token, profile and email endpoints of one provider.
// The token endpoint refuses a code it did not issue and a verifier that does
// not match the one the test used, like a real provider would.
func fakeProvider(t *testing.T, name, profile string, emailVerified bool, wantVerifier string) (Provider, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("client_secret") != "secret" {
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") != wantVerifier ||
			r.Form.Get("redirect_uri") != "https://api.test/v0/auth/callback" {
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "bearer"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(profile))
	})
	mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "old@example.com", "primary": false, "verified": true},
			{"email": "person@example.com", "primary": true, "verified": emailVerified},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return Provider{Name: name, ClientID: "id", ClientSecret: "secret",
		AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/token", UserURL: srv.URL + "/user",
		EmailsURL: srv.URL + "/emails", Scopes: []string{"openid", "email"}}, srv
}

const callback = "https://api.test/v0/auth/callback"

func TestAuthorizeURLCarriesPKCE(t *testing.T) {
	p, _ := fakeProvider(t, "google", `{}`, true, rfcVerifier)
	raw := p.AuthorizeURL("state-1", ChallengeS256(rfcVerifier), callback)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for key, want := range map[string]string{
		"client_id":             "id",
		"response_type":         "code",
		"state":                 "state-1",
		"redirect_uri":          callback,
		"code_challenge":        ChallengeS256(rfcVerifier),
		"code_challenge_method": "S256",
		"scope":                 "openid email",
	} {
		if q.Get(key) != want {
			t.Fatalf("%s = %q, want %q", key, q.Get(key), want)
		}
	}
	if strings.Contains(raw, rfcVerifier) {
		t.Fatal("the verifier must never travel in the authorize URL")
	}
}

func TestCodeGoogleIdentity(t *testing.T) {
	p, srv := fakeProvider(t, "google",
		`{"sub":"1234","email":"Person@Example.com","email_verified":true,"name":"Ada","picture":"https://img/1"}`, true, rfcVerifier)
	id, err := p.Code(context.Background(), srv.Client(), "good-code", rfcVerifier, callback)
	if err != nil {
		t.Fatal(err)
	}
	if id.ProviderID != "1234" || id.Email != "Person@Example.com" || !id.EmailVerified || id.Name != "Ada" {
		t.Fatalf("identity = %+v", id)
	}
	if id.Provider != "google" {
		t.Fatalf("provider = %q", id.Provider)
	}
}

func TestCodeRejectsBadCode(t *testing.T) {
	p, srv := fakeProvider(t, "google", `{}`, true, rfcVerifier)
	if _, err := p.Code(context.Background(), srv.Client(), "replayed", rfcVerifier, callback); !errors.Is(err, ErrCodeRejected) {
		t.Fatalf("replayed code: err = %v", err)
	}
	if _, err := p.Code(context.Background(), srv.Client(), "good-code", "wrong-verifier-of-the-right-length-00000000000", callback); !errors.Is(err, ErrCodeRejected) {
		t.Fatalf("wrong verifier: err = %v", err)
	}
}

func TestCodeGitHubUsesVerifiedEmail(t *testing.T) {
	profile := `{"id":42,"login":"ada","name":"","email":null,"avatar_url":"https://img/2"}`
	p, srv := fakeProvider(t, "github", profile, false, rfcVerifier)
	if _, err := p.Code(context.Background(), srv.Client(), "good-code", rfcVerifier, callback); !errors.Is(err, ErrEmailUnverified) {
		t.Fatalf("an unverified primary address must be refused, err = %v", err)
	}

	p2, srv2 := fakeProvider(t, "github", profile, true, rfcVerifier)
	id, err := p2.Code(context.Background(), srv2.Client(), "good-code", rfcVerifier, callback)
	if err != nil {
		t.Fatal(err)
	}
	if id.ProviderID != "42" || id.Email != "person@example.com" || !id.EmailVerified || id.Name != "ada" {
		t.Fatalf("identity = %+v", id)
	}
}

func TestCodeProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	p := Provider{Name: "google", ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL, UserURL: srv.URL}
	if _, err := p.Code(context.Background(), srv.Client(), "good-code", rfcVerifier, callback); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestProviderConfigured(t *testing.T) {
	if (Provider{ClientID: "id"}).Configured() {
		t.Fatal("a provider without a secret is not configured")
	}
	if !Google("id", "secret").Configured() || !GitHub("id", "secret").Configured() {
		t.Fatal("Google and GitHub with credentials must be configured")
	}
	if Google("", "").Configured() {
		t.Fatal("Google without credentials must not be configured")
	}
}

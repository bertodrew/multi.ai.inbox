package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

func quietServer(t *testing.T, svc *core.Service) *Server {
	t.Helper()
	return &Server{Svc: svc, Log: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
		Auth: AuthSurface{WebOrigin: "https://site.test", PublicURL: "https://api.test"}}
}

func do(t *testing.T, s *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("body is not the error envelope: %v (%s)", err, w.Body.String())
	}
	return e.Code
}

// Without MAI_AUTH_SECRET every sign in entry point answers 501 and says so,
// instead of pretending to work. API keys are untouched by that switch.
func TestAuthEndpointsDisabledWithoutSecret(t *testing.T) {
	s := quietServer(t, &core.Service{})
	if w := do(t, s, "POST", "/v0/auth/magic-link", `{"email":"person@example.com"}`, nil); w.Code != http.StatusNotImplemented {
		t.Fatalf("magic link: %d %s", w.Code, w.Body.String())
	} else if code := errorCode(t, w); code != "auth_disabled" {
		t.Fatalf("magic link code = %q", code)
	}
	if w := do(t, s, "POST", "/v0/auth/refresh", `{"refresh_token":"x"}`, nil); w.Code != http.StatusNotImplemented {
		t.Fatalf("refresh: %d", w.Code)
	}
	if w := do(t, s, "GET", "/v0/auth/oauth/google/start", "", nil); w.Code != http.StatusNotImplemented {
		t.Fatalf("oauth start: %d", w.Code)
	}
}

func TestAuthEndpointsArePublic(t *testing.T) {
	// No Authorization header anywhere: a 401 here would mean the route fell
	// through to the API key middleware.
	s := quietServer(t, &core.Service{})
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/v0/auth/magic-link", `{"email":"person@example.com"}`},
		{"POST", "/v0/auth/refresh", `{"refresh_token":"x"}`},
		{"POST", "/v0/auth/logout", `{"refresh_token":"x"}`},
		{"GET", "/v0/auth/callback?token=x", ""},
		{"GET", "/v0/auth/oauth/github/start", ""},
	} {
		w := do(t, s, tc.method, tc.path, tc.body, map[string]string{"Content-Type": "application/json"})
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("%s %s asked for an API key: %s", tc.method, tc.path, w.Body.String())
		}
	}
}

func TestRefreshRejectsMalformedToken(t *testing.T) {
	svc := &core.Service{Auth: &core.AuthConfig{Secret: []byte(strings.Repeat("k", 32))}}
	s := quietServer(t, svc)
	w := do(t, s, "POST", "/v0/auth/refresh", `{"refresh_token":"junk"}`, map[string]string{"Content-Type": "application/json"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if code := errorCode(t, w); code != "invalid_refresh_token" {
		t.Fatalf("code = %q", code)
	}
}

func TestCallbackWithoutParameters(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "GET", "/v0/auth/callback", "", map[string]string{"Accept": "application/json"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if code := errorCode(t, w); code != "validation_error" {
		t.Fatalf("code = %q", code)
	}
}

// A browser gets a redirect back to the login page with an error code, never a
// JSON body it would render.
func TestCallbackRedirectsBrowser(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "GET", "/v0/auth/callback", "", map[string]string{"Accept": "text/html"})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "https://site.test/login/?error=validation_error" {
		t.Fatalf("location = %q", loc)
	}
}

// The magic link form is a plain browser post, so the endpoint accepts a form
// body and answers with a redirect to the login page.
func TestMagicLinkFormPost(t *testing.T) {
	svc := &core.Service{Auth: &core.AuthConfig{Secret: []byte(strings.Repeat("k", 32))}}
	s := quietServer(t, svc)
	w := do(t, s, "POST", "/v0/auth/magic-link", "email=person@example.com&redirect_after=%2Flogin%2F",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	// No relay is configured on this service, so delivery is reported as
	// unavailable; what matters is that the form was read and the answer is a
	// redirect rather than a JSON error page.
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "https://site.test/login/?error=auth_disabled" {
		t.Fatalf("location = %q", loc)
	}
}

func TestMeRequiresCredential(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "GET", "/v0/me", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
	if code := errorCode(t, w); code != "missing_authorization" {
		t.Fatalf("code = %q", code)
	}
}

func TestSafeRedirectPathsOnly(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "POST", "/v0/auth/magic-link", "email=person@example.com&redirect_after=%2F%2Fevil.example",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if loc := w.Header().Get("Location"); strings.Contains(loc, "evil.example") {
		t.Fatalf("open redirect: %q", loc)
	}
}

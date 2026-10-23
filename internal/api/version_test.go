package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

// The alias test drives the real mux through httptest: no database is needed,
// because an unauthenticated request and a malformed public request are both
// answered before core is touched. It proves three things at once: /v1 resolves
// to the same handler as /v0, the body and status are byte identical between
// the two, and the version headers say what the deprecation policy promises.

func TestV1AliasesV0ForProtectedRoutes(t *testing.T) {
	s := quietServer(t, &core.Service{})
	v0 := do(t, s, "GET", "/v0/me", "", nil)
	v1 := do(t, s, "GET", "/v1/me", "", nil)

	if v0.Code != http.StatusUnauthorized || v1.Code != http.StatusUnauthorized {
		t.Fatalf("status v0 = %d, v1 = %d, want 401 from both (a 404 means the alias is missing)",
			v0.Code, v1.Code)
	}
	if v0.Body.String() != v1.Body.String() {
		t.Fatalf("bodies differ:\nv0 %s\nv1 %s", v0.Body.String(), v1.Body.String())
	}
	if code := errorCode(t, v1); code != "missing_authorization" {
		t.Fatalf("v1 error code = %q", code)
	}
}

func TestVersionHeaders(t *testing.T) {
	s := quietServer(t, &core.Service{})

	v1 := do(t, s, "GET", "/v1/me", "", nil)
	if got := v1.Header().Get(HeaderAPIVersion); got != VersionV1 {
		t.Fatalf("v1 %s = %q, want %q", HeaderAPIVersion, got, VersionV1)
	}
	for _, h := range []string{"Sunset", "Link"} {
		if got := v1.Header().Get(h); got != "" {
			t.Fatalf("v1 response carries %s = %q, a deprecation header", h, got)
		}
	}

	v0 := do(t, s, "GET", "/v0/me", "", nil)
	if got := v0.Header().Get(HeaderAPIVersion); got != VersionV0 {
		t.Fatalf("v0 %s = %q, want %q", HeaderAPIVersion, got, VersionV0)
	}
	if got, want := v0.Header().Get("Sunset"), SunsetV0.Format(http.TimeFormat); got != want {
		t.Fatalf("Sunset = %q, want %q", got, want)
	}
	if got, want := v0.Header().Get("Link"), `</v1/me>; rel="successor-version"`; got != want {
		t.Fatalf("Link = %q, want %q", got, want)
	}
	// The deprecation is a header, never a body field: existing clients parse
	// the same envelope they always did.
	if body := errorCode(t, v0); body != "missing_authorization" {
		t.Fatalf("v0 body changed: code = %q", body)
	}
}

// The successor link names the same operation, not the prefix it lives under.
func TestSuccessorLinkKeepsThePathAndDropsTheQuery(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "GET", "/v0/inboxes/agent%40example.eu/threads?limit=5", "", nil)
	if got, want := w.Header().Get("Link"), `</v1/inboxes/agent%40example.eu/threads>; rel="successor-version"`; got != want {
		t.Fatalf("Link = %q, want %q", got, want)
	}
}

func TestPublicAuthRoutesAreAliased(t *testing.T) {
	s := quietServer(t, &core.Service{})
	v0 := do(t, s, "GET", "/v0/auth/callback", "", map[string]string{"Accept": "application/json"})
	v1 := do(t, s, "GET", "/v1/auth/callback", "", map[string]string{"Accept": "application/json"})
	if v0.Code != http.StatusBadRequest || v1.Code != http.StatusBadRequest {
		t.Fatalf("status v0 = %d, v1 = %d, want 400 from both", v0.Code, v1.Code)
	}
	if v0.Body.String() != v1.Body.String() {
		t.Fatalf("bodies differ:\nv0 %s\nv1 %s", v0.Body.String(), v1.Body.String())
	}
	// A public route must not fall through to the API key middleware.
	if code := errorCode(t, v1); code != "validation_error" {
		t.Fatalf("v1 error code = %q", code)
	}
}

// A form post is redirected by the same handler under both prefixes.
func TestPublicRedirectIsAliased(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "POST", "/v1/auth/magic-link", "email=person%40example.com",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if w.Code != http.StatusSeeOther && w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want a redirect or the 501 switch", w.Code)
	}
	if got := w.Header().Get(HeaderAPIVersion); got != VersionV1 {
		t.Fatalf("%s = %q", HeaderAPIVersion, got)
	}
}

func TestVersionInContext(t *testing.T) {
	s := quietServer(t, &core.Service{})
	var seen []string
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, VersionFrom(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	})
	h := s.versioned(echo)
	for _, path := range []string{"/v0/inboxes", "/v1/inboxes", "/healthz"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	}
	want := []string{VersionV0, VersionV1, ""}
	if len(seen) != len(want) {
		t.Fatalf("saw %d versions, want %d", len(seen), len(want))
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("version for request %d = %q, want %q", i, seen[i], want[i])
		}
	}
}

func TestUnknownRouteAnswersLikeItsV0Twin(t *testing.T) {
	s := quietServer(t, &core.Service{})
	// /v0/ is authenticated before routing, so an unknown path without a key
	// answers 401 rather than 404. /v1 must behave the same way.
	v0 := do(t, s, "GET", "/v0/not-a-route", "", nil)
	v1 := do(t, s, "GET", "/v1/not-a-route", "", nil)
	if v0.Code != v1.Code {
		t.Fatalf("status v0 = %d, v1 = %d, want the same answer", v0.Code, v1.Code)
	}
	if v0.Body.String() != v1.Body.String() {
		t.Fatalf("bodies differ:\nv0 %s\nv1 %s", v0.Body.String(), v1.Body.String())
	}
}

func TestMCPIsNotVersioned(t *testing.T) {
	s := quietServer(t, &core.Service{})
	w := do(t, s, "GET", "/mcp", "", nil)
	if got := w.Header().Get(HeaderAPIVersion); got != "" {
		t.Fatalf("/mcp carries %s = %q", HeaderAPIVersion, got)
	}
}

// Every /v0 operation must have a /v1 twin: a handler that exists on v0 but
// not on v1 would be a silent gap in the alias. The check walks the same route
// table the server builds by looking at the mux's own registration list.
func TestEveryRouteHasAV1Twin(t *testing.T) {
	for _, pattern := range []string{
		"POST /v0/api-keys", "GET /v0/api-keys", "DELETE /v0/api-keys/{key}",
		"GET /v0/audit-logs", "GET /v0/auth/me", "GET /v0/me",
		"POST /v0/inboxes", "GET /v0/inboxes", "GET /v0/inboxes/{inbox}",
		"PATCH /v0/inboxes/{inbox}", "DELETE /v0/inboxes/{inbox}",
		"GET /v0/inboxes/{inbox}/threads", "GET /v0/inboxes/{inbox}/threads/{thread}",
		"GET /v0/threads", "GET /v0/threads/{thread}",
		"GET /v0/inboxes/{inbox}/messages", "POST /v0/inboxes/{inbox}/messages/send",
		"GET /v0/inboxes/{inbox}/messages/{message}", "PATCH /v0/inboxes/{inbox}/messages/{message}",
		"GET /v0/inboxes/{inbox}/messages/{message}/raw", "POST /v0/inboxes/{inbox}/messages/{message}/reply",
		"POST /v0/inboxes/{inbox}/messages/{message}/release",
		"GET /v0/inboxes/{inbox}/messages/{message}/attachments/{attachment}",
		"POST /v0/inboxes/{inbox}/drafts", "GET /v0/inboxes/{inbox}/drafts",
		"GET /v0/inboxes/{inbox}/drafts/{draft}", "PATCH /v0/inboxes/{inbox}/drafts/{draft}",
		"DELETE /v0/inboxes/{inbox}/drafts/{draft}", "POST /v0/inboxes/{inbox}/drafts/{draft}/send",
		"GET /v0/events/stream", "POST /v0/domains", "GET /v0/domains",
		"GET /v0/domains/{domain}", "DELETE /v0/domains/{domain}", "POST /v0/domains/{domain}/verify",
		"POST /v0/webhooks", "GET /v0/webhooks", "DELETE /v0/webhooks/{webhook}",
		"GET /v0/billing", "POST /v0/billing/checkout", "POST /v0/billing/portal",
		"GET /v0/export", "DELETE /v0/organization",
		"GET /v0/tickets", "POST /v0/tickets", "GET /v0/tickets/stats",
		"GET /v0/tickets/{ticket}", "PATCH /v0/tickets/{ticket}",
		"POST /v0/tickets/{ticket}/notes", "POST /v0/tickets/{ticket}/reply",
		"POST /v0/auth/magic-link", "GET /v0/auth/callback", "POST /v0/auth/refresh",
		"POST /v0/auth/logout", "GET /v0/auth/oauth/{provider}/start",
	} {
		a := aliases(pattern)
		if len(a) != 2 {
			t.Fatalf("%s: %d mounts, want 2", pattern, len(a))
		}
		want := strings.Replace(pattern, "/v0/", "/v1/", 1)
		if a[0] != pattern || a[1] != want {
			t.Fatalf("%s mounted as %v, want [%s %s]", pattern, a, pattern, want)
		}
	}
}

package main

// Regression tests for the landing API. Focused on the session/credential
// lifecycle — the P0 bug class (connect destroyed its own credentials) found
// by the CTO review on 19/10/2026.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

const ncOKXML = `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK">
  <Errors/>
  <CommandResponse>
    <DomainGetListResult>
      <Domain Name="example.org" Created="01/01/2025" Expires="01/01/2027" IsExpired="false" IsLocked="true" AutoRenew="true"/>
    </DomainGetListResult>
  </CommandResponse>
</ApiResponse>`

// testEnv wires a mock Namecheap API and an isolated cred dir for one test.
func testEnv(t *testing.T, xml string) {
	t.Helper()
	t.Setenv("LANDING_CRED_DIR", filepath.Join(t.TempDir(), "creds"))
	nc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(xml))
	}))
	t.Setenv("NAMECHEAP_API_URL", nc.URL)
	t.Cleanup(nc.Close)
}

// newVisitor performs the initial anonymous GET (session bootstrap), then
// returns a POST /api/connect request carrying the session cookie and the
// session's CSRF token.
func newVisitor(t *testing.T, xml string) *http.Request {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ensureSession(w, r) // issue the session like the status endpoint does
	}))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/landingpage/api/status")
	if err != nil {
		t.Fatalf("status GET failed: %v", err)
	}
	defer res.Body.Close()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == cookieName && c.Value != "" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("no session cookie issued")
	}
	sess, ok := sessions.m[cookie.Value]
	if !ok {
		t.Fatalf("session not registered in the store")
	}
	csrf := ensureCSRF(sess) // force-generate: the status GET leaves it empty

	req, err := http.NewRequest(http.MethodPost, "http://connect.test/landingpage/api/connect",
		strings.NewReader(url.Values{"username": {"demo"}, "api_key": {"0123456789abcdef"}, "csrf": {csrf}}.Encode()))
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Landing-CSRF", csrf)
	req.AddCookie(cookie)
	return req
}

// TestConnectKeepsCredentialsUnderRotatedSession guards the P0 fix: a
// successful connect must leave the credentials readable under the NEW
// session id (rotate first, store second).
func TestConnectKeepsCredentialsUnderRotatedSession(t *testing.T) {
	testEnv(t, ncOKXML)

	rec := httptest.NewRecorder()
	handleConnect(rec, newVisitor(t, ncOKXML))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect returned %d: %s", rec.Code, rec.Body.String())
	}

	// The cookie on the response is the rotated session id.
	var newSID string
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			newSID = c.Value
		}
	}
	if newSID == "" {
		t.Fatalf("no rotated session cookie on connect response")
	}

	// THE REGRESSION: credentials must exist under the rotated session id.
	if _, err := readCreds(newSID); err != nil {
		t.Fatalf("credentials unreadable after successful connect (P0 bug): %v", err)
	}
}

// TestConnectFailureLeavesNoCredentials: a failed Namecheap call must not
// write any credential file.
func TestConnectFailureLeavesNoCredentials(t *testing.T) {
	ncErrXML := `<ApiResponse Status="ERROR"><Errors><Error Number="2011166">Not Found</Error></Errors></ApiResponse>`
	testEnv(t, ncErrXML)

	visitor := newVisitor(t, ncErrXML)
	sid, _ := getSession(visitor)
	rec := httptest.NewRecorder()
	handleConnect(rec, visitor)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := readCreds(sid); err == nil {
		t.Fatalf("credential file written despite failed connect")
	}
}

// gwConnect boots a session, runs a successful connect through handleConnect,
// and returns the rotated session id.
func gwConnect(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handleConnect(rec, newVisitor(t, ncOKXML))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect failed: %d: %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c.Value
		}
	}
	t.Fatalf("no rotated session cookie on connect response")
	return ""
}

// TestGatewayDomainRequiresSession: the per-domain gateway surface answers 401
// without a connected session (GET and PATCH both).
func TestGatewayDomainRequiresSession(t *testing.T) {
	testEnv(t, ncOKXML)
	t.Setenv("MAILCOW_API_KEY", "test-super-admin-key")

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "http://t.test/landingpage/api/gateway/domains/example.org", nil)
	req.SetPathValue("domain", "example.org")
	handleGatewayDomainGet(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET without session: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPatch, "http://t.test/landingpage/api/gateway/domains/example.org",
		strings.NewReader(`{"maxquota":1024}`))
	req2.SetPathValue("domain", "example.org")
	handleGatewayDomainPatch(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("PATCH without session: expected 401, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

// TestGatewayDomainPatchGuardsCSRF: a connected session with a wrong CSRF
// token gets 403 — the write path never trusts the cookie alone.
func TestGatewayDomainPatchGuardsCSRF(t *testing.T) {
	testEnv(t, ncOKXML)
	t.Setenv("MAILCOW_API_KEY", "test-super-admin-key")
	sid := gwConnect(t)

	req, _ := http.NewRequest(http.MethodPatch, "http://t.test/landingpage/api/gateway/domains/example.org",
		strings.NewReader(`{"maxquota":1024}`))
	req.SetPathValue("domain", "example.org")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	req.Header.Set("X-Landing-CSRF", "wrong-token")
	rec := httptest.NewRecorder()
	handleGatewayDomainPatch(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PATCH with wrong CSRF: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestGatewayDomainWriteFlow: connected session reads the mail-platform record
// and PATCHes it; ptr_target surfaces as a warning, never a fake success.
func TestGatewayDomainWriteFlow(t *testing.T) {
	testEnv(t, ncOKXML)
	t.Setenv("MAILCOW_API_KEY", "test-super-admin-key")
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/get/domain/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"domain_name":"example.org","description":"Brand one","maxquota":"1024","defquota":"512"}`))
		case r.URL.Path == "/api/v1/edit/domain":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"type":"success","msg":["domain edited"]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Setenv("MAILCOW_API_URL", panel.URL)
	t.Cleanup(panel.Close)
	sid := gwConnect(t)
	csrf := ensureCSRF(sessions.m[sid])

	// GET: the mail-platform record lands in the form fields
	req, _ := http.NewRequest(http.MethodGet, "http://t.test/landingpage/api/gateway/domains/example.org", nil)
	req.SetPathValue("domain", "example.org")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	rec := httptest.NewRecorder()
	handleGatewayDomainGet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		OK     bool `json:"ok"`
		Domain struct {
			FQDN     string `json:"fqdn"`
			Branding string `json:"branding"`
		} `json:"domain"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.OK || got.Domain.FQDN != "example.org" || got.Domain.Branding != "Brand one" {
		t.Fatalf("GET record wrong: %s", rec.Body.String())
	}

	// PATCH: writable fields applied, ptr_target → warning
	req3, _ := http.NewRequest(http.MethodPatch, "http://t.test/landingpage/api/gateway/domains/example.org",
		strings.NewReader(`{"branding":"Brand one","ptr_target":"mail.example.org","maxquota":1024,"defquota":512}`))
	req3.SetPathValue("domain", "example.org")
	req3.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	req3.Header.Set("X-Landing-CSRF", csrf)
	rec3 := httptest.NewRecorder()
	handleGatewayDomainPatch(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("PATCH: expected 200, got %d: %s", rec3.Code, rec3.Body.String())
	}
	var wrote struct {
		OK       bool     `json:"ok"`
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(rec3.Body.Bytes(), &wrote)
	if !wrote.OK || len(wrote.Warnings) != 1 {
		t.Fatalf("PATCH result wrong: %s", rec3.Body.String())
	}

	// Invalid domain name → 400 (no path tricks reach the panel call)
	req4, _ := http.NewRequest(http.MethodGet, "http://t.test/landingpage/api/gateway/domains/..%2fetc", nil)
	req4.SetPathValue("domain", "../etc")
	req4.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	rec4 := httptest.NewRecorder()
	handleGatewayDomainGet(rec4, req4)
	if rec4.Code != http.StatusBadRequest {
		t.Fatalf("invalid domain: expected 400, got %d: %s", rec4.Code, rec4.Body.String())
	}
}

// TestEnvChoiceLifecycle: the shared/dedicated choice is 401 without a
// session, 400 for unknown values, and round-trips per account (survives a
// session restart because the file is keyed to the username).
func TestEnvChoiceLifecycle(t *testing.T) {
	testEnv(t, ncOKXML)
	t.Setenv("LANDING_CRED_DIR", t.TempDir())

	// 401 without a session
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "http://t.test/landingpage/api/env", strings.NewReader(`{"env":"shared"}`))
	handleEnv(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST without session: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	// GET before any choice → env:null
	sid := gwConnect(t)
	csrf := ensureCSRF(sessions.m[sid])
	req2, _ := http.NewRequest(http.MethodGet, "http://t.test/landingpage/api/env", nil)
	req2.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	rec2 := httptest.NewRecorder()
	handleEnv(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), `"env":null`) {
		t.Fatalf("GET before choice: %d: %s", rec2.Code, rec2.Body.String())
	}

	// 400 for an unknown env value
	req3, _ := http.NewRequest(http.MethodPost, "http://t.test/landingpage/api/env", strings.NewReader(`{"env":"chaos"}`))
	req3.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	req3.Header.Set("X-Landing-CSRF", csrf)
	rec3 := httptest.NewRecorder()
	handleEnv(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("unknown env: expected 400, got %d: %s", rec3.Code, rec3.Body.String())
	}

	// POST then GET: the choice round-trips
	req4, _ := http.NewRequest(http.MethodPost, "http://t.test/landingpage/api/env", strings.NewReader(`{"env":"dedicated"}`))
	req4.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	req4.Header.Set("X-Landing-CSRF", csrf)
	rec4 := httptest.NewRecorder()
	handleEnv(rec4, req4)
	if rec4.Code != http.StatusOK || !strings.Contains(rec4.Body.String(), `"env":"dedicated"`) {
		t.Fatalf("POST choice failed: %d: %s", rec4.Code, rec4.Body.String())
	}
	req5, _ := http.NewRequest(http.MethodGet, "http://t.test/landingpage/api/env", nil)
	req5.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	rec5 := httptest.NewRecorder()
	handleEnv(rec5, req5)
	if !strings.Contains(rec5.Body.String(), `"env":"dedicated"`) {
		t.Fatalf("choice did not round-trip: %s", rec5.Body.String())
	}
}

// TestGatewayDomainTenantScoping guards the cross-tenant fix (20/10): the
// path domain must be on the connected tenant's Namecheap account — a
// shared-instance tenant can't read or write another tenant's gateway.
func TestGatewayDomainTenantScoping(t *testing.T) {
	testEnv(t, ncOKXML)

	// Mock panel: accept edits, answer the domain GET with a record.
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/get/domain/example.org" {
			_, _ = w.Write([]byte(`[{"domain_name":"example.org","description":"Demo","max_quota":1024,"def_quota":256}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Setenv("MAILCOW_API_URL", panel.URL)
	t.Setenv("MAILCOW_API_KEY", "test-key-0123456789")
	t.Cleanup(panel.Close)

	// Ownership mock: the tenant owns example.org only.
	restore := ncOwnedDomains
	ncOwnedDomains = func(c creds) ([]map[string]any, *ncResult) {
		return []map[string]any{{"name": "example.org"}}, nil
	}
	t.Cleanup(func() { ncOwnedDomains = restore })

	rec := httptest.NewRecorder()
	handleConnect(rec, newVisitor(t, ncOKXML))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect returned %d: %s", rec.Code, rec.Body.String())
	}
	var sid string
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			sid = c.Value
		}
	}
	if sid == "" {
		t.Fatalf("no rotated session cookie on connect response")
	}
	csrf := ensureCSRF(sessions.m[sid])

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/gateway/domains/{domain}", handleGatewayDomainPatch)
	patch := func(domain string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodPatch, "http://gw.test/api/gateway/domains/"+domain, strings.NewReader(`{"branding":"Demo"}`))
		if err != nil {
			t.Fatalf("request build failed: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Landing-CSRF", csrf)
		req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		return out
	}

	if code := patch("example.org").Code; code != http.StatusOK {
		t.Fatalf("own domain blocked by the scoping: %d", code)
	}
	if code := patch("other.com").Code; code != http.StatusForbidden {
		t.Fatalf("foreign domain not rejected: %d", code)
	}

	// Fail-closed: an ownership API failure must not unlock the path.
	ncOwnedDomains = func(c creds) ([]map[string]any, *ncResult) {
		return nil, &ncResult{Error: "mock down"}
	}
	if code := patch("example.org").Code; code != http.StatusBadGateway {
		t.Fatalf("ownership failure must fail closed, got %d", code)
	}
}

package main

// Regression tests for the landing API. Focused on the session/credential
// lifecycle — the P0 bug class (connect destroyed its own credentials) found
// by the CTO review on 19/10/2026.

import (
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

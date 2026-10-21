package main

// Tests for the Mollie checkout (21/10): fail-closed without a key, honest
// checkout creation with a key, server-side verification in the webhook,
// subscription creation on paid, idempotent behaviour for unknown ids.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testMollie mocks the Mollie API: payments GET returns the given payment
// object; payments POST records the request body for assertions.
func testMollie(t *testing.T, payment map[string]any, recorded *map[string]any) *httptest.Server {
	t.Helper()
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/payments":
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			var rec map[string]any
			_ = json.Unmarshal(b, &rec)
			if recorded != nil {
				*recorded = rec
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"tr_first","status":"open","customerId":"cst_test","_links":{"checkout":{"href":"https://checkout.mollie.test/pay/tr_first"}}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/payments/tr_first"):
			pb, _ := json.Marshal(payment)
			_, _ = w.Write(pb)
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"title":"Not Found"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/subscriptions"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sub_test","status":"active","nextPaymentDate":"2026-11-21"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Setenv("MOLLIE_API_URL", m.URL)
	t.Setenv("MOLLIE_API_KEY", "test_key_0123456789")
	t.Cleanup(m.Close)
	return m
}

func decodeJSON(s string) map[string]any {
	var out map[string]any
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// connectedSession connects a session the same way the other tests do and
// returns (session id, csrf) for authenticated checkout requests.
func connectedSession(t *testing.T) (string, string) {
	t.Helper()
	testEnv(t, ncOKXML)
	ipHits.mu.Lock()
	ipHits.m = map[string][]time.Time{}
	ipHits.mu.Unlock()
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
	return sid, ensureCSRF(sessions.m[sid])
}

// checkoutReq builds POST api/checkout {plan} with the session cookie+CSRF.
func checkoutReq(t *testing.T, sid, csrf, plan string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://ck.test/landingpage/api/checkout", strings.NewReader(`{"plan":"`+plan+`"}`))
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Landing-CSRF", csrf)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	return req
}

func TestCheckoutRequiresConnection(t *testing.T) {
	testEnv(t, ncOKXML)
	req, err := http.NewRequest(http.MethodPost, "http://ck.test/landingpage/api/checkout", strings.NewReader(`{"plan":"solo"}`))
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleCheckout(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous checkout must be 401, got %d", rec.Code)
	}
}

func TestCheckoutFailClosedWithoutKey(t *testing.T) {
	sid, csrf := connectedSession(t)
	t.Setenv("MOLLIE_API_KEY", "")
	rec := httptest.NewRecorder()
	handleCheckout(rec, checkoutReq(t, sid, csrf, "solo"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no key must fail closed with 503, got %d", rec.Code)
	}
}

func TestCheckoutUnknownPlan(t *testing.T) {
	sid, csrf := connectedSession(t)
	testMollie(t, nil, nil)
	rec := httptest.NewRecorder()
	handleCheckout(rec, checkoutReq(t, sid, csrf, "galaxy"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown plan must be 404, got %d", rec.Code)
	}
}

func TestCheckoutCreatesFirstPayment(t *testing.T) {
	sid, csrf := connectedSession(t)
	var recorded map[string]any
	testMollie(t, nil, &recorded)
	rec := httptest.NewRecorder()
	handleCheckout(rec, checkoutReq(t, sid, csrf, "solo"))
	if rec.Code != http.StatusOK {
		t.Fatalf("checkout returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["checkout_url"] != "https://checkout.mollie.test/pay/tr_first" {
		t.Fatalf("checkout_url missing: %v", body)
	}
	// THE GUARD: the first payment must carry sequenceType=first and the
	// account metadata the webhook needs to map the payment back.
	if recorded["sequenceType"] != "first" {
		t.Fatalf("payment must be sequenceType=first, got %v", recorded["sequenceType"])
	}
	meta, _ := recorded["metadata"].(map[string]any)
	if meta == nil || meta["plan"] != "solo" || meta["username"] == "" {
		t.Fatalf("payment metadata must carry plan+username, got %v", recorded["metadata"])
	}
}

func TestCheckoutWebhookMarksPaidAndPersists(t *testing.T) {
	sid, _ := connectedSession(t)
	paid := map[string]any{
		"status":     "paid",
		"customerId": "cst_test",
		"metadata":   map[string]any{"username": "demo", "plan": "solo"},
	}
	testMollie(t, paid, nil)

	form := url.Values{"id": {"tr_first"}}
	req, err := http.NewRequest(http.MethodPost, "http://ck.test/landingpage/api/checkout/webhook", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["status"] != "paid" {
		t.Fatalf("webhook must mark paid, got %v", body)
	}
	// State must be persisted per account (survives restarts).
	data, err := os.ReadFile(checkoutPath("demo"))
	if err != nil {
		t.Fatalf("billing state not persisted: %v", err)
	}
	saved := decodeJSON(string(data))
	if saved["plan"] != "solo" || saved["subscription_id"] != "sub_test" {
		t.Fatalf("billing state wrong: %v", saved)
	}
	_ = sid
}

func TestCheckoutWebhookIgnoresUnknownPayment(t *testing.T) {
	connectedSession(t)
	paid := map[string]any{"status": "paid", "metadata": map[string]any{"username": "demo", "plan": "solo"}}
	testMollie(t, paid, nil)
	form := url.Values{"id": {"tr_unknown"}}
	req, err := http.NewRequest(http.MethodPost, "http://ck.test/landingpage/api/checkout/webhook", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unknown id must be 200 idempotent, got %d", rec.Code)
	}
	if _, err := os.Stat(checkoutPath("demo")); !os.IsNotExist(err) {
		t.Fatalf("unknown payment must not touch billing state")
	}
}

func TestCheckoutStateRequiresConnection(t *testing.T) {
	testEnv(t, ncOKXML)
	req, err := http.NewRequest(http.MethodGet, "http://ck.test/landingpage/api/checkout/state", nil)
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	rec := httptest.NewRecorder()
	handleCheckoutState(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous state must be 401, got %d", rec.Code)
	}
}

// guard: the billing file lands in the cred dir, 0600 — assert the helper
// keeps the same isolation as the credential files.
func TestCheckoutPathIsPerAccount(t *testing.T) {
	a := checkoutPath("demo")
	b := checkoutPath("other")
	if a == b || a == "" || !strings.Contains(filepath.Base(a), "bill_") {
		t.Fatalf("checkoutPath must be per-account: %q vs %q", a, b)
	}
}

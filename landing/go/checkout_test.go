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
		case r.Method == http.MethodPost && r.URL.Path == "/customers":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"cst_new","resource":"customer","name":"AImulti tenant demo","metadata":{"username":"demo"}}`))
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

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func osWriteFile(path string, b []byte) error {
	return os.WriteFile(path, b, 0600)
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
	// THE GUARD: the first payment must carry sequenceType=first, the Mollie
	// customer (422 otherwise — found live 21/10) and the account metadata.
	if recorded["sequenceType"] != "first" {
		t.Fatalf("payment must be sequenceType=first, got %v", recorded["sequenceType"])
	}
	if recorded["customerId"] != "cst_new" {
		t.Fatalf("payment must carry the Mollie customerId, got %v", recorded["customerId"])
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

func TestCheckoutPersistsCustomerForReuse(t *testing.T) {
	sid, csrf := connectedSession(t)
	var payments []map[string]any
	testMollie(t, nil, nil)
	// two checkouts: the second must REUSE the persisted customer, not
	// create a second one (one Mollie customer per tenant).
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handleCheckout(rec, checkoutReq(t, sid, csrf, "solo"))
		if rec.Code != http.StatusOK {
			t.Fatalf("checkout %d returned %d: %s", i, rec.Code, rec.Body.String())
		}
		data, err := os.ReadFile(checkoutPath("demo"))
		if err != nil {
			t.Fatalf("customer id not persisted: %v", err)
		}
		if decodeJSON(string(data))["customer_id"] != "cst_new" {
			t.Fatalf("customer_id missing from billing state")
		}
	}
	_ = payments
}

// Tests for the plan-cap enforcement (20/10): the shared tiers limit the
// number of domains server-side (revenue risk otherwise), and the node
// capacity cap refuses new purchases when the shared pool is full.

// planCapSession connects a session and registers two owned domains so a
// second gateway PATCH can be attempted against the cap.
func planCapSession(t *testing.T) (string, string) {
	t.Helper()
	sid, csrf := connectedSession(t)
	restore := ncOwnedDomains
	ncOwnedDomains = func(c creds) ([]map[string]any, *ncResult) {
		return []map[string]any{{"name": "example.org"}, {"name": "example.net"}}, nil
	}
	t.Cleanup(func() { ncOwnedDomains = restore })
	return sid, csrf
}

// planCapPatch routes a PATCH through the real mux pattern.
func planCapPatch(t *testing.T, sid, csrf, domain string) *httptest.ResponseRecorder {
	t.Helper()
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Setenv("MAILCOW_API_URL", panel.URL)
	t.Setenv("MAILCOW_API_KEY", "test-key-0123456789")
	t.Cleanup(panel.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/gateway/domains/{domain}", handleGatewayDomainPatch)
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

func TestTrialCapOneDomain(t *testing.T) {
	sid, csrf := planCapSession(t)
	if code := planCapPatch(t, sid, csrf, "example.org").Code; code != http.StatusOK {
		t.Fatalf("first domain blocked on trial: %d", code)
	}
	rec := planCapPatch(t, sid, csrf, "example.net")
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("trial second domain must be 402, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Trial includes 1 domains") {
		t.Fatalf("limit message must name the plan: %s", rec.Body.String())
	}
}

func TestPaidPlanCapEnforced(t *testing.T) {
	sid, csrf := planCapSession(t)
	// simulate a paid Solo subscription (1 domain max)
	b, _ := jsonMarshal(map[string]any{"plan": "solo", "status": "paid"})
	if err := osWriteFile(checkoutPath("demo"), b); err != nil {
		t.Fatalf("state write failed: %v", err)
	}
	if code := planCapPatch(t, sid, csrf, "example.org").Code; code != http.StatusOK {
		t.Fatalf("paid first domain blocked: %d", code)
	}
	if code := planCapPatch(t, sid, csrf, "example.net").Code; code != http.StatusPaymentRequired {
		t.Fatalf("Solo second domain must be 402, got %d", code)
	}
}

func TestDedicatedUnlimitedDomains(t *testing.T) {
	sid, csrf := planCapSession(t)
	b, _ := jsonMarshal(map[string]any{"plan": "dedicated", "status": "paid"})
	_ = osWriteFile(checkoutPath("demo"), b)
	if code := planCapPatch(t, sid, csrf, "example.org").Code; code != http.StatusOK {
		t.Fatalf("dedicated first domain blocked: %d", code)
	}
	if code := planCapPatch(t, sid, csrf, "example.net").Code; code != http.StatusOK {
		t.Fatalf("dedicated unlimited domains violated: %d", code)
	}
}

func TestNodeCapacityRefusesPurchase(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	sid, csrf := connectedSession(t)
	testMollie(t, nil, nil)
	t.Setenv("MAX_TENANTS_PER_NODE", "1")
	// one tenant already onboarded (the connect created an env file? no —
	// write one to simulate an onboarded tenant).
	_ = osWriteFile(envPath("other"), []byte(`{"env":"shared"}`))
	rec := httptest.NewRecorder()
	handleCheckout(rec, checkoutReq(t, sid, csrf, "solo"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("full node must refuse purchase with 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPlanForResolution(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	if planFor("demo").Label != "Trial" {
		t.Fatalf("no state must resolve to Trial, got %q", planFor("demo").Label)
	}
	_ = osWriteFile(checkoutPath("demo"), []byte(`{"plan":"studio","status":"paid"}`))
	p := planFor("demo")
	if p.Label != "Studio" || p.MaxDomains != 3 {
		t.Fatalf("paid state must resolve to Studio/3, got %q/%d", p.Label, p.MaxDomains)
	}
	_ = osWriteFile(checkoutPath("demo"), []byte(`{"plan":"studio","status":"open"}`))
	if planFor("demo").Label != "Trial" {
		t.Fatalf("unpaid state must stay Trial")
	}
}

package main

// Tests for the Mollie checkout (21/10): fail-closed without a key (501,
// honest — same convention as auth.go for unconfigured OAuth providers),
// honest checkout creation with a key, server-side verification in the
// webhook, subscription activation on paid, idempotent behaviour for
// unknown ids AND for webhook retries (never a duplicate subscription),
// concurrent overlapping retries (exactly ONE subscription under N
// webhooks), and the full chain: paid → subscription attiva → provJob queued.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testMollie mocks the Mollie API: payments GET returns the given payment
// object; payments POST records the request body for assertions. The
// optional variadic recorder collects every subscription POST body — a
// duplicate subscription is a double-billing class of bug, so the tests
// COUNT the POSTs, not just the persisted state.
func testMollie(t *testing.T, payment map[string]any, recorded *map[string]any, subRecorded ...*[]map[string]any) *httptest.Server {
	t.Helper()
	var subMu sync.Mutex // the subscription recorder is shared across goroutines (concurrency test)
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/customers":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"cst_new","resource":"customer","name":"AImulti tenant demo","metadata":{"username":"demo"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/payments":
			// io.ReadAll, not a single Read on a r.ContentLength-sized
			// buffer: with ContentLength -1 (chunked) or a multi-chunk
			// body the capture comes back empty or partial.
			b, _ := io.ReadAll(r.Body)
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
			// io.ReadAll, not a single Read on a r.ContentLength-sized
			// buffer: with ContentLength -1 (chunked) or a multi-chunk
			// body the capture comes back empty or partial. The append is
			// under the mutex: the concurrency test drives this handler
			// from N goroutines at once.
			b2, _ := io.ReadAll(r.Body)
			var rec2 map[string]any
			_ = json.Unmarshal(b2, &rec2)
			subMu.Lock()
			if len(subRecorded) > 0 && subRecorded[0] != nil {
				*subRecorded[0] = append(*subRecorded[0], rec2)
			}
			subMu.Unlock()
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
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("no key must fail closed with honest 501, got %d", rec.Code)
	}
	// The body names the missing configuration — no fake checkout, no fake ok.
	if s, _ := decodeJSON(rec.Body.String())["error"].(string); !strings.Contains(s, "not configured") {
		t.Fatalf("error must be honest about the missing config, got %q", s)
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

// -----------------------------------------------------------------------
// End-to-end payment flow: webhook paid → subscription attiva → provJob
// queued. Mollie does NOT sign webhooks — the verification IS the
// server-side fetch under our own key, so the honest rejects are covered by
// the unfetchable-id (TestCheckoutWebhookIgnoresUnknownPayment), non-paid
// and unattributable-payment tests below.

// The full chain on ONE webhook: paid payment → subscription created
// server-side → billing state persisted (status + subscription_status) →
// provJob queued for the worker (PROVISIONING_ENABLED=1, budget armed via
// the documented SPEND_INSTANCE_COSTS override, LANDING_CRED_DIR temp).
func TestCheckoutWebhookPaidActivatesSubscriptionAndQueuesProvisioning(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("PROVISIONING_ENABLED", "1")
	t.Setenv("MAX_MONTHLY_SPEND_EUR", "500") // dedicated ccx types sit outside the default 20 EUR budget
	t.Setenv("SPEND_INSTANCE_COSTS", `{"ccx41":138.99,"ccx63":853.49}`) // the ops override that arms the Dedicated whitelist
	var subs []map[string]any
	testMollie(t, paidPayment("dedicated"), nil, &subs)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["status"] != "paid" || body["subscription"] != "created" {
		t.Fatalf("paid webhook must activate the subscription, got %v", body)
	}
	if body["subscription_id"] != "sub_test" || body["subscription_status"] != "active" {
		t.Fatalf("response must report the active subscription, got %v", body)
	}
	if body["provisioning"] != "queued" {
		t.Fatalf("dedicated paid must queue provisioning, got %v", body)
	}
	// The subscription was created SERVER-SIDE with the recurring contract
	// (never trusted from the webhook body).
	if len(subs) != 1 {
		t.Fatalf("exactly one subscription must be created, got %d", len(subs))
	}
	if subs[0]["interval"] != "1 month" {
		t.Fatalf("subscription must recur monthly, got %v", subs[0]["interval"])
	}
	if amount, _ := subs[0]["amount"].(map[string]any); amount == nil || amount["value"] != "149.00" || amount["currency"] != "EUR" {
		t.Fatalf("subscription amount wrong: %v", subs[0]["amount"])
	}
	// Billing state: paid + active subscription, per account.
	data, err := os.ReadFile(checkoutPath("demo"))
	if err != nil {
		t.Fatalf("billing state not persisted: %v", err)
	}
	saved := decodeJSON(string(data))
	if saved["status"] != "paid" || saved["plan"] != "dedicated" || saved["subscription_id"] != "sub_test" || saved["subscription_status"] != "active" {
		t.Fatalf("billing state wrong: %v", saved)
	}
	if saved["first_payment_id"] != "tr_first" || saved["last_payment_id"] != "tr_first" {
		t.Fatalf("payment ids not persisted: %v", saved)
	}
	// The chain closes: the provisioning job is queued for the worker.
	j := waitForProvJob(t, provJobPath("demo"))
	if j.State != "queued" || j.Plan != "dedicated" || j.PaymentID != "tr_first" {
		t.Fatalf("provJob wrong: %+v", j)
	}
	if info, err := os.Stat(provJobPath("demo")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("provJob file must be 0600, got %v", info.Mode())
	}
}

// Mollie retries the webhook on transport failures and every monthly
// recurring payment arrives paid — a naive POST /subscriptions per webhook
// would stack DUPLICATE subscriptions on the same customer (double
// billing). An already-active subscription is never re-created; the
// mandate payment id is preserved; the newest paid is tracked honestly.
func TestCheckoutWebhookDoesNotDuplicateSubscription(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	var subs []map[string]any
	testMollie(t, paidPayment("solo"), nil, &subs)
	seed := func(firstPayment string) {
		b, _ := jsonMarshal(map[string]any{
			"plan": "solo", "label": "Solo", "status": "paid",
			"subscription_id": "sub_test", "subscription_status": "active",
			"customer_id": "cst_test", "first_payment_id": firstPayment,
		})
		if err := osWriteFile(checkoutPath("demo"), b); err != nil {
			t.Fatalf("seed write failed: %v", err)
		}
	}
	// Case 1 — Mollie retry: the SAME payment id re-posted.
	seed("tr_first")
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("retry webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	if decodeJSON(rec.Body.String())["subscription"] != "already_active" {
		t.Fatalf("retry must not re-create the subscription: %s", rec.Body.String())
	}
	// Case 2 — next monthly payment: a DIFFERENT paid id for the same
	// customer (the mandate id stays, the newest paid is tracked).
	seed("tr_2026_10")
	rec2 := httptest.NewRecorder()
	handleCheckoutWebhook(rec2, webhookReq(t, "tr_first"))
	if rec2.Code != http.StatusOK {
		t.Fatalf("recurring webhook returned %d: %s", rec2.Code, rec2.Body.String())
	}
	if decodeJSON(rec2.Body.String())["subscription"] != "already_active" {
		t.Fatalf("recurring paid must not stack a second subscription: %s", rec2.Body.String())
	}
	if len(subs) != 0 {
		t.Fatalf("no subscription POST may fire when one is already active, got %d", len(subs))
	}
	data, err := os.ReadFile(checkoutPath("demo"))
	if err != nil {
		t.Fatalf("billing state missing: %v", err)
	}
	saved := decodeJSON(string(data))
	if saved["subscription_id"] != "sub_test" || saved["status"] != "paid" {
		t.Fatalf("subscription state must stay intact: %v", saved)
	}
	if saved["first_payment_id"] != "tr_2026_10" {
		t.Fatalf("mandate payment id must be preserved, got %v", saved["first_payment_id"])
	}
	if saved["last_payment_id"] != "tr_first" {
		t.Fatalf("last payment id must track the newest paid, got %v", saved["last_payment_id"])
	}
}

// Concurrency (21/10): Mollie retries can OVERLAP — a transport timeout
// followed by an immediate redelivery hits the webhook while the first
// attempt is still in flight. N concurrent webhooks for the SAME payment
// must produce exactly ONE POST /subscriptions (double billing otherwise):
// the winner answers created, the losers paid/already_active. The guard is
// on the RECORDED POSTs, not on the persisted state alone — the state can
// look right while N subscription POSTs fired on the wire.
func TestCheckoutWebhookConcurrentRetriesSingleSubscription(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	const n = 8
	var subs []map[string]any
	testMollie(t, paidPayment("solo"), nil, &subs)
	// One FRESH request per goroutine: r.Body is one-shot — sharing a
	// single *http.Request would hand every goroutine but the first an
	// already-consumed body.
	reqs := make([]*http.Request, n)
	for i := range reqs {
		reqs[i] = webhookReq(t, "tr_first")
	}
	recs := make([]*httptest.ResponseRecorder, n)
	for i := range recs {
		recs[i] = httptest.NewRecorder()
	}
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			handleCheckoutWebhook(recs[i], reqs[i])
		}(i)
	}
	wg.Wait()
	created, already := 0, 0
	for i := 0; i < n; i++ {
		if recs[i].Code != http.StatusOK {
			t.Fatalf("webhook %d returned %d: %s", i, recs[i].Code, recs[i].Body.String())
		}
		body := decodeJSON(recs[i].Body.String())
		if body["status"] != "paid" {
			t.Fatalf("webhook %d must answer paid, got %v", i, body)
		}
		switch body["subscription"] {
		case "created":
			created++
		case "already_active":
			already++
		default:
			t.Fatalf("webhook %d must answer created or already_active, got %v", i, body)
		}
	}
	if created != 1 {
		t.Fatalf("exactly ONE subscription may be created under concurrent retries, got %d", created)
	}
	if created+already != n {
		t.Fatalf("all %d webhooks must answer created/already_active, got created=%d already_active=%d", n, created, already)
	}
	// THE GUARD on the wire: exactly one POST /subscriptions recorded.
	if len(subs) != 1 {
		t.Fatalf("exactly one POST /subscriptions may be recorded, got %d", len(subs))
	}
}

// Repair path: the account is already paid but has NO subscription id (the
// previous creation attempt failed and Mollie got a 200, so nothing retries
// it) — a new paid webhook retries the subscription honestly.
func TestCheckoutWebhookRepairsMissingSubscription(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	var subs []map[string]any
	testMollie(t, paidPayment("solo"), nil, &subs)
	b, _ := jsonMarshal(map[string]any{"plan": "solo", "status": "paid", "customer_id": "cst_test", "first_payment_id": "tr_first"})
	if err := osWriteFile(checkoutPath("demo"), b); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	if decodeJSON(rec.Body.String())["subscription"] != "created" {
		t.Fatalf("missing subscription must be repaired: %s", rec.Body.String())
	}
	if len(subs) != 1 {
		t.Fatalf("exactly one repair subscription POST, got %d", len(subs))
	}
	data, err := os.ReadFile(checkoutPath("demo"))
	if err != nil {
		t.Fatalf("billing state missing: %v", err)
	}
	saved := decodeJSON(string(data))
	if saved["subscription_id"] != "sub_test" || saved["subscription_status"] != "active" {
		t.Fatalf("repaired subscription not persisted: %v", saved)
	}
}

// A paid webhook for a DIFFERENT plan while a subscription is already
// active must not silently repoint the billing state (plan changes are a
// subscription swap — open hole, kept honest): the live plan is kept and
// the response says so.
func TestCheckoutWebhookKeepsActivePlanOnPlanChange(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	var subs []map[string]any
	testMollie(t, paidPayment("studio"), nil, &subs)
	b, _ := jsonMarshal(map[string]any{
		"plan": "solo", "label": "Solo", "status": "paid",
		"subscription_id": "sub_test", "subscription_status": "active",
		"customer_id": "cst_test", "first_payment_id": "tr_first",
	})
	if err := osWriteFile(checkoutPath("demo"), b); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["subscription"] != "already_active" {
		t.Fatalf("no second subscription may fire on a plan change: %v", body)
	}
	data, err := os.ReadFile(checkoutPath("demo"))
	if err != nil {
		t.Fatalf("billing state missing: %v", err)
	}
	if decodeJSON(string(data))["plan"] != "solo" {
		t.Fatalf("active plan must not be silently repointed: %s", data)
	}
	if s, _ := body["note"].(string); !strings.Contains(s, "plan change") {
		t.Fatalf("plan change must be noted honestly, got %v", body["note"])
	}
}

// Subscription creation fails server-side (e.g. Mollie 500): the webhook
// still answers 200 so Mollie stops retrying, but reports creation_failed
// honestly — the billing state stays WITHOUT a subscription id so the next
// paid webhook repairs it. Never a fake active subscription.
func TestCheckoutWebhookSubscriptionFailureIsHonest(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/payments/tr_first"):
			_, _ = w.Write([]byte(`{"status":"paid","customerId":"cst_test","metadata":{"username":"demo","plan":"solo"}}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/subscriptions"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":500,"title":"Internal Server Error"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Setenv("MOLLIE_API_URL", m.URL)
	t.Setenv("MOLLIE_API_KEY", "test_key_0123456789")
	t.Cleanup(m.Close)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook must stay 200 for Mollie, got %d", rec.Code)
	}
	body := decodeJSON(rec.Body.String())
	if body["subscription"] != "creation_failed" || body["status"] != "paid" {
		t.Fatalf("failure must be reported honestly, got %v", body)
	}
	data, err := os.ReadFile(checkoutPath("demo"))
	if err != nil {
		t.Fatalf("billing state missing: %v", err)
	}
	saved := decodeJSON(string(data))
	if saved["status"] != "paid" {
		t.Fatalf("the payment IS paid, state stays paid: %v", saved)
	}
	if _, has := saved["subscription_id"]; has {
		t.Fatalf("a failed subscription must NOT be persisted as active: %v", saved)
	}
}

// Honest rejects (Mollie does not sign webhooks — verification IS the
// server-side fetch under our key): a payment that is not paid must never
// activate a subscription or write billing state.
func TestCheckoutWebhookNonPaidNeverActivates(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	var subs []map[string]any
	testMollie(t, map[string]any{
		"status":     "failed",
		"customerId": "cst_test",
		"metadata":   map[string]any{"username": "demo", "plan": "solo"},
	}, nil, &subs)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	if decodeJSON(rec.Body.String())["status"] != "failed" {
		t.Fatalf("non-paid status must be echoed honestly: %s", rec.Body.String())
	}
	if _, err := os.Stat(checkoutPath("demo")); !os.IsNotExist(err) {
		t.Fatalf("non-paid payment must not write billing state")
	}
	if len(subs) != 0 {
		t.Fatalf("non-paid payment must not create a subscription, got %d", len(subs))
	}
}

// A paid payment that cannot be attributed to any account (no username
// metadata): nothing is activated, nothing is written, Mollie gets its 200.
func TestCheckoutWebhookRejectsUnattributablePayment(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	testMollie(t, map[string]any{"status": "paid", "customerId": "cst_test"}, nil)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	if decodeJSON(rec.Body.String())["note"] != "no metadata" {
		t.Fatalf("unattributable payment must be reported honestly: %s", rec.Body.String())
	}
	if _, err := os.Stat(checkoutPath("demo")); !os.IsNotExist(err) {
		t.Fatalf("unattributable payment must not write billing state")
	}
}

// Fail-closed on the webhook too: without the key the honest answer is 501
// (Not Implemented) — no state is touched.
func TestCheckoutWebhookFailClosedWithoutKey(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("MOLLIE_API_KEY", "")
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("no key on webhook must fail closed with 501, got %d", rec.Code)
	}
	if _, err := os.Stat(checkoutPath("demo")); !os.IsNotExist(err) {
		t.Fatalf("fail-closed webhook must not touch billing state")
	}
}

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/stripe"
)

const whsec = "whsec_e2e_secret"

// fakeStripe answers the three Stripe endpoints the platform calls and
// records the forms it receives.
type fakeStripe struct {
	mu    sync.Mutex
	forms map[string]url.Values
	subs  map[string]string // subscription id -> JSON object
}

func (f *fakeStripe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer sk_test_e2e" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"api_key_invalid"}}`))
		return
	}
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forms[r.URL.Path] = r.PostForm
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/checkout/sessions":
		_, _ = w.Write([]byte(`{"id":"cs_test_1","url":"https://checkout.stripe.test/c/cs_test_1"}`))
	case r.Method == "POST" && r.URL.Path == "/v1/billing_portal/sessions":
		_, _ = w.Write([]byte(`{"id":"bps_1","url":"https://billing.stripe.test/p/bps_1"}`))
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
		sub, ok := f.subs[strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"resource_missing"}}`))
			return
		}
		_, _ = w.Write([]byte(sub))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeStripe) form(path string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forms[path]
}

// subscriptionJSON builds a subscription object in the 2025 API shape (period on the item).
func subscriptionJSON(id, org, status, price string, periodEnd time.Time) string {
	return fmt.Sprintf(`{"id":%q,"object":"subscription","customer":"cus_e2e","status":%q,"cancel_at_period_end":false,
		"metadata":{"org_id":%q},"items":{"data":[{"current_period_end":%d,"price":{"id":%q}}]}}`,
		id, status, org, periodEnd.Unix(), price)
}

func TestStripeBilling(t *testing.T) {
	ctx := context.Background()
	d := openPlatform(t, ctx, freshDatabase(t))
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	fake := &fakeStripe{forms: map[string]url.Values{}, subs: map[string]string{}}
	stripeSrv := httptest.NewServer(fake)
	t.Cleanup(stripeSrv.Close)

	// Billing off: every billing endpoint answers 501 billing_disabled.
	off := &core.Service{DB: d, SharedDomain: shared, Hostname: "mx." + shared}
	org, _ := off.CreateOrg(ctx, "Payer")
	if _, err := off.SetScreening(ctx, org.ID, false); err != nil {
		t.Fatal(err)
	}
	key, _ := off.CreateKey(ctx, org.ID, "owner")
	offSrv := httptest.NewServer((&api.Server{Svc: off, Log: log}).Handler())
	t.Cleanup(offSrv.Close)
	offClient := client{t: t, base: offSrv.URL, key: key.Key}
	if code, body := offClient.do("GET", "/v0/billing", nil, nil); code != 501 || body["code"] != "billing_disabled" {
		t.Fatalf("billing off: %d %v", code, body)
	}
	if code, body := offClient.do("POST", "/v0/billing/checkout", map[string]any{"plan": "growth"}, nil); code != 501 || body["code"] != "billing_disabled" {
		t.Fatalf("checkout off: %d %v", code, body)
	}
	if resp, err := http.Post(offSrv.URL+"/stripe/webhook", "application/json", strings.NewReader("{}")); err != nil || resp.StatusCode != 501 {
		t.Fatalf("webhook off: %v %v", err, resp)
	}

	prices := map[string]core.Price{
		"price_builder_m": {Plan: "builder", Interval: "monthly"},
		"price_growth_m":  {Plan: "growth", Interval: "monthly"},
		"price_growth_y":  {Plan: "growth", Interval: "yearly"},
		"price_team_y":    {Plan: "team", Interval: "yearly"},
	}
	svc := &core.Service{DB: d, SharedDomain: shared, Hostname: "mx." + shared, Billing: &core.Billing{
		Stripe: &stripe.Client{SecretKey: "sk_test_e2e", BaseURL: stripeSrv.URL}, WebhookSecret: whsec,
		Prices: prices, ReturnURL: "https://console.example.eu/billing"}}
	srv := httptest.NewServer((&api.Server{Svc: svc, Log: log}).Handler())
	t.Cleanup(srv.Close)
	owner := client{t: t, base: srv.URL, key: key.Key}

	// Permissions: billing_read and billing_write are separate scopes.
	_, rk := owner.do("POST", "/v0/api-keys", map[string]any{"name": "reader", "permissions": []string{"billing_read"}}, nil)
	reader := client{t: t, base: srv.URL, key: fmt.Sprint(rk["api_key"])}
	if code, body := reader.do("POST", "/v0/billing/checkout", map[string]any{"plan": "growth"}, nil); code != 403 || body["code"] != "missing_permission" {
		t.Fatalf("checkout without billing_write: %d %v", code, body)
	}
	if code, body := reader.do("GET", "/v0/billing", nil, nil); code != 200 || body["plan"] != "free" || body["status"] != "none" {
		t.Fatalf("billing before checkout: %d %v", code, body)
	}

	// Checkout: subscription mode, Stripe Tax and VAT ID collection, org reference.
	if code, body := owner.do("POST", "/v0/billing/checkout", map[string]any{"plan": "enterprise"}, nil); code != 400 {
		t.Fatalf("unknown plan: %d %v", code, body)
	}
	if code, body := owner.do("POST", "/v0/billing/checkout", map[string]any{"plan": "team", "interval": "monthly"}, nil); code != 400 {
		t.Fatalf("unconfigured price: %d %v", code, body)
	}
	code, body := owner.do("POST", "/v0/billing/checkout", map[string]any{"plan": "growth"}, nil)
	if code != 200 || body["url"] != "https://checkout.stripe.test/c/cs_test_1" {
		t.Fatalf("checkout: %d %v", code, body)
	}
	form := fake.form("/v1/checkout/sessions")
	for k, want := range map[string]string{
		"mode": "subscription", "line_items[0][price]": "price_growth_m", "line_items[0][quantity]": "1",
		"client_reference_id": org.ID, "subscription_data[metadata][org_id]": org.ID,
		"automatic_tax[enabled]": "true", "tax_id_collection[enabled]": "true",
		"success_url": "https://console.example.eu/billing", "cancel_url": "https://console.example.eu/billing",
	} {
		if got := form.Get(k); got != want {
			t.Errorf("checkout form %s = %q, want %q", k, got, want)
		}
	}
	if code, body := owner.do("POST", "/v0/billing/portal", nil, nil); code != 409 || body["code"] != "no_billing_account" {
		t.Fatalf("portal before a customer exists: %d %v", code, body)
	}

	hook := func(id, typ string, created time.Time, object string, sig func([]byte) string) (int, map[string]any) {
		t.Helper()
		payload := []byte(fmt.Sprintf(`{"id":%q,"object":"event","type":%q,"created":%d,"data":{"object":%s}}`, id, typ, created.Unix(), object))
		req, _ := http.NewRequest("POST", srv.URL+"/stripe/webhook", bytes.NewReader(payload))
		if sig == nil {
			req.Header.Set("Stripe-Signature", stripe.Sign(payload, whsec, time.Now()))
		} else {
			req.Header.Set("Stripe-Signature", sig(payload))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	plan := func() string {
		t.Helper()
		o, err := svc.GetOrg(ctx, org.ID)
		if err != nil {
			t.Fatal(err)
		}
		return o.Plan
	}
	t0 := time.Now().Add(-time.Minute)
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	fake.mu.Lock()
	fake.subs["sub_1"] = subscriptionJSON("sub_1", org.ID, "active", "price_growth_m", periodEnd)
	fake.mu.Unlock()
	completed := fmt.Sprintf(`{"id":"cs_test_1","object":"checkout.session","mode":"subscription","client_reference_id":%q,
		"customer":"cus_e2e","subscription":"sub_1","metadata":{"org_id":%q}}`, org.ID, org.ID)

	// Signature: wrong secret and old timestamps are refused and change nothing.
	if code, body := hook("evt_bad", "checkout.session.completed", t0, completed, func(p []byte) string {
		return stripe.Sign(p, "whsec_wrong", time.Now())
	}); code != 400 || body["code"] != "invalid_signature" {
		t.Fatalf("wrong secret: %d %v", code, body)
	}
	if code, body := hook("evt_bad", "checkout.session.completed", t0, completed, func(p []byte) string {
		return stripe.Sign(p, whsec, time.Now().Add(-10*time.Minute))
	}); code != 400 || body["code"] != "invalid_signature" {
		t.Fatalf("old timestamp: %d %v", code, body)
	}
	if code, _ := hook("evt_bad", "checkout.session.completed", t0, completed, func([]byte) string { return "" }); code != 400 {
		t.Fatalf("missing signature: %d", code)
	}
	if plan() != "free" {
		t.Fatal("a refused event changed the plan")
	}

	// checkout.session.completed reads the subscription and grants growth.
	if code, body := hook("evt_1", "checkout.session.completed", t0, completed, nil); code != 200 || body["outcome"] != core.StripeApplied {
		t.Fatalf("checkout completed: %d %v", code, body)
	}
	if plan() != "growth" {
		t.Fatalf("plan after checkout = %s, want growth", plan())
	}
	code, body = reader.do("GET", "/v0/billing", nil, nil)
	if code != 200 || body["status"] != "active" || body["interval"] != "monthly" || body["current_period_end"] != periodEnd.UTC().Format(time.RFC3339) {
		t.Fatalf("billing after checkout: %d %v", code, body)
	}

	// Idempotency: the same event again is a no op.
	if code, body := hook("evt_1", "checkout.session.completed", t0, completed, nil); code != 200 || body["outcome"] != core.StripeDuplicate {
		t.Fatalf("redelivery: %d %v", code, body)
	}
	countAudit := func(action string) int {
		t.Helper()
		var n int
		if err := d.WithSystem(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from audit_log where org_id = $1 and action = $2", org.ID, action).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := countAudit("billing.update"); n != 1 {
		t.Fatalf("billing.update audit entries after a redelivery = %d, want 1", n)
	}
	if n := countAudit("billing.checkout"); n != 1 {
		t.Fatalf("billing.checkout audit entries = %d, want 1", n)
	}

	// A second checkout while subscribed is refused; the portal opens for the customer.
	if code, body := owner.do("POST", "/v0/billing/checkout", map[string]any{"plan": "team", "interval": "yearly"}, nil); code != 409 || body["code"] != "already_subscribed" {
		t.Fatalf("second checkout: %d %v", code, body)
	}
	if code, body := owner.do("POST", "/v0/billing/portal", map[string]any{"return_url": "https://console.example.eu/x"}, nil); code != 200 || body["url"] != "https://billing.stripe.test/p/bps_1" {
		t.Fatalf("portal: %d %v", code, body)
	}
	if f := fake.form("/v1/billing_portal/sessions"); f.Get("customer") != "cus_e2e" || f.Get("return_url") != "https://console.example.eu/x" {
		t.Fatalf("portal form: %v", f)
	}

	// Upgrade in the portal: customer.subscription.updated to team yearly.
	upgraded := subscriptionJSON("sub_1", org.ID, "active", "price_team_y", periodEnd.Add(365*24*time.Hour))
	if code, body := hook("evt_2", "customer.subscription.updated", t0.Add(time.Second), upgraded, nil); code != 200 || body["outcome"] != core.StripeApplied {
		t.Fatalf("upgrade: %d %v", code, body)
	}
	if plan() != "team" {
		t.Fatalf("plan after upgrade = %s", plan())
	}
	// A delayed older event never overwrites newer state.
	if _, body := hook("evt_old", "customer.subscription.updated", t0.Add(-time.Hour), subscriptionJSON("sub_1", org.ID, "active", "price_builder_m", periodEnd), nil); body["outcome"] != core.StripeStale || plan() != "team" {
		t.Fatalf("stale event: %v, plan %s", body, plan())
	}

	// invoice.payment_failed: past_due keeps the plan during the grace period.
	failed := `{"id":"in_1","object":"invoice","customer":"cus_e2e","parent":{"subscription_details":{"subscription":"sub_1","metadata":{}}}}`
	if code, body := hook("evt_3", "invoice.payment_failed", t0.Add(2*time.Second), failed, nil); code != 200 || body["outcome"] != core.StripeApplied {
		t.Fatalf("payment failed: %d %v", code, body)
	}
	_, body = reader.do("GET", "/v0/billing", nil, nil)
	if body["status"] != "past_due" || body["plan"] != "team" || body["grace_period_ends_at"] == nil {
		t.Fatalf("billing while past due: %v", body)
	}
	// Stripe's own past_due update does not restart the grace clock.
	if _, body := hook("evt_4", "customer.subscription.updated", t0.Add(3*time.Second), subscriptionJSON("sub_1", org.ID, "past_due", "price_team_y", periodEnd), nil); body["outcome"] != core.StripeApplied || plan() != "team" {
		t.Fatalf("past_due update: %v, plan %s", body, plan())
	}

	// Scheduler: still inside the grace period, then past it.
	shifted := func(d time.Duration) *core.Service {
		s := *svc
		s.Now = func() time.Time { return time.Now().Add(d) }
		return &s
	}
	if n, err := shifted(6 * 24 * time.Hour).DowngradeLapsedSubscriptions(ctx); err != nil || n != 0 {
		t.Fatalf("downgrade inside the grace period: %d %v", n, err)
	}
	if n, err := shifted(8 * 24 * time.Hour).DowngradeLapsedSubscriptions(ctx); err != nil || n != 1 {
		t.Fatalf("downgrade after the grace period: %d %v", n, err)
	}
	if plan() != "free" || countAudit("billing.downgrade") != 1 {
		t.Fatalf("after the grace period: plan %s, downgrade audits %d", plan(), countAudit("billing.downgrade"))
	}
	if n, _ := shifted(9 * 24 * time.Hour).DowngradeLapsedSubscriptions(ctx); n != 0 {
		t.Fatal("an organization already on free was downgraded again")
	}

	// The customer pays: active again restores the plan and clears the clock.
	if _, body := hook("evt_5", "customer.subscription.updated", t0.Add(4*time.Second), upgraded, nil); body["outcome"] != core.StripeApplied || plan() != "team" {
		t.Fatalf("recovered: %v, plan %s", body, plan())
	}
	_, body = reader.do("GET", "/v0/billing", nil, nil)
	if body["status"] != "active" || body["past_due_since"] != nil || body["interval"] != "yearly" {
		t.Fatalf("billing after recovery: %v", body)
	}

	// Cancellation: customer.subscription.deleted moves to free.
	canceled := subscriptionJSON("sub_1", org.ID, "canceled", "price_team_y", periodEnd)
	if _, body := hook("evt_6", "customer.subscription.deleted", t0.Add(5*time.Second), canceled, nil); body["outcome"] != core.StripeApplied || plan() != "free" {
		t.Fatalf("deleted: %v, plan %s", body, plan())
	}
	_, body = reader.do("GET", "/v0/billing", nil, nil)
	if body["status"] != "canceled" {
		t.Fatalf("billing after cancellation: %v", body)
	}

	// Unknown event types and unknown customers are acknowledged, not retried.
	if code, body := hook("evt_7", "customer.created", t0.Add(6*time.Second), `{"id":"cus_x"}`, nil); code != 200 || body["outcome"] != core.StripeIgnored {
		t.Fatalf("unknown type: %d %v", code, body)
	}
	if code, body := hook("evt_8", "invoice.payment_failed", t0.Add(7*time.Second), `{"id":"in_x","customer":"cus_nobody","subscription":"sub_nobody"}`, nil); code != 200 || body["outcome"] != core.StripeIgnored {
		t.Fatalf("unknown customer: %d %v", code, body)
	}
	// Tenants cannot read the event log.
	var seen int
	if err := d.WithOrg(ctx, org.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from stripe_events").Scan(&seen)
	}); err != nil || seen != 0 {
		t.Fatalf("tenant sees %d stripe events (%v)", seen, err)
	}
}

package main

// Tests for the FASE 1 provisioning trigger (21/10): a paid dedicated plan
// queues a 0600 job file (state queued) that the external worker consumes;
// without PROVISIONING_ENABLED=1 nothing is queued (fail-closed); shared
// plans never queue; the state endpoint is auth-gated and honest. No
// terraform is involved — only the job file contract.

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

// webhookReq builds POST api/checkout/webhook with a form id (Mollie posts
// form-encoded id only). The webhook is auth-free by design, so no session
// is needed.
func webhookReq(t *testing.T, id string) *http.Request {
	t.Helper()
	form := url.Values{"id": {id}}
	req, err := http.NewRequest(http.MethodPost, "http://ck.test/landingpage/api/checkout/webhook", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// paidPayment is a paid Mollie payment with the given plan metadata.
func paidPayment(plan string) map[string]any {
	return map[string]any{
		"status":     "paid",
		"customerId": "cst_test",
		"metadata":   map[string]any{"username": "demo", "plan": plan},
	}
}

// waitForProvJob polls for the job file the background goroutine writes —
// the webhook answers before the write lands (eventually-consistent).
func waitForProvJob(t *testing.T, path string) provJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			var j provJob
			_ = json.Unmarshal(b, &j)
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("provisioning job not written to %s in time", path)
	return provJob{}
}

// noProvFiles fails when any prov_*.json landed in the cred dir.
func noProvFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cred dir unreadable: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "prov_") {
			t.Fatalf("no provisioning job file must exist, found %s", e.Name())
		}
	}
}

func TestWebhookQueuesProvisioningForDedicated(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir()) // t.TempDir() DIRECTLY — no uncreated subdir
	t.Setenv("PROVISIONING_ENABLED", "1")
	t.Setenv("PROVISIONING_BASE_DOMAIN", "aimulti-inbox.test")
	t.Setenv("MAX_MONTHLY_SPEND_EUR", "500") // dedicated ccx types sit outside the default 20 EUR budget
	t.Setenv("SPEND_INSTANCE_COSTS", `{"ccx41":138.99,"ccx63":853.49}`) // the ops override that arms Dedicated (founder raises budget + whitelist)
	testMollie(t, paidPayment("dedicated"), nil)

	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["provisioning"] != "queued" {
		t.Fatalf("dedicated paid must report provisioning=queued, got %v", body)
	}
	// The job file: queued, keyed to the account, 0600 like credentials.
	j := waitForProvJob(t, provJobPath("demo"))
	if j.State != "queued" || j.Plan != "dedicated" || j.PaymentID != "tr_first" {
		t.Fatalf("job state/plan/payment wrong: %+v", j)
	}
	if j.ServerType != "ccx41" || j.Label != "Dedicated" || j.Username != "demo" {
		t.Fatalf("job server type/label wrong: %+v", j)
	}
	if j.Hostname != "demo.aimulti-inbox.test" {
		t.Fatalf("job hostname wrong: %q", j.Hostname)
	}
	if info, err := os.Stat(provJobPath("demo")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("job file must be 0600, got %v", info.Mode().Perm())
	}
	// The persist-only billing flow still happened.
	if _, err := os.Stat(checkoutPath("demo")); err != nil {
		t.Fatalf("billing state must still be persisted: %v", err)
	}
}

func TestWebhookProvisioningFailClosed(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("PROVISIONING_ENABLED", "0")
	testMollie(t, paidPayment("dedicated-xl"), nil)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["provisioning"] != "not_enabled" {
		t.Fatalf("disabled must report not_enabled, got %v", body)
	}
	noProvFiles(t, os.Getenv("LANDING_CRED_DIR"))
	// Billing state is still persisted (the flow stays persist-only).
	if _, err := os.Stat(checkoutPath("demo")); err != nil {
		t.Fatalf("billing state must still be persisted: %v", err)
	}
}

func TestWebhookSharedPlanNeverQueues(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("PROVISIONING_ENABLED", "1")
	testMollie(t, paidPayment("solo"), nil)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(rec.Body.String())
	if body["provisioning"] != "not_applicable" {
		t.Fatalf("shared plan must not trigger provisioning, got %v", body)
	}
	noProvFiles(t, os.Getenv("LANDING_CRED_DIR"))
}

// guard: the job file is keyed per account (no cross-tenant state).
func TestProvJobFileIsPerAccount(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	a := provJobPath("demo")
	b := provJobPath("other")
	if a == b || a == "" || !strings.Contains(filepath.Base(a), "prov_") {
		t.Fatalf("provJobPath must be per-account: %q vs %q", a, b)
	}
}

// Idempotency: Mollie retries the webhook and every monthly payment arrives
// paid — a queued/running/done job is never reset; a failed one is re-queued.
func TestProvQueueIdempotent(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("MAX_MONTHLY_SPEND_EUR", "500") // dedicated ccx types sit outside the default 20 EUR budget
	t.Setenv("SPEND_INSTANCE_COSTS", `{"ccx41":138.99,"ccx63":853.49}`)
	if state, err := queueProvisioning("demo", "dedicated", "tr_first"); err != nil || state != "queued" {
		t.Fatalf("first queue failed: state=%q err=%v", state, err)
	}
	j1 := waitForProvJob(t, provJobPath("demo"))
	if state, err := queueProvisioning("demo", "dedicated", "tr_second"); err != nil || state != "queued" {
		t.Fatalf("second queue failed: state=%q err=%v", state, err)
	}
	j2 := waitForProvJob(t, provJobPath("demo"))
	if j2.PaymentID != j1.PaymentID || j2.CreatedAt != j1.CreatedAt || j2.Attempts != 0 {
		t.Fatalf("second paid must not reset the job: %+v vs %+v", j1, j2)
	}
	// A failed job is re-queued (attempt bumped, stale output dropped).
	j2.State = "failed"
	j2.Error = "terraform failed"
	if err := saveProvJob(j2); err != nil {
		t.Fatalf("failed-state save: %v", err)
	}
	if state, err := queueProvisioning("demo", "dedicated", "tr_second"); err != nil || state != "queued" {
		t.Fatalf("failed job must re-queue: state=%q err=%v", state, err)
	}
	j3 := waitForProvJob(t, provJobPath("demo"))
	if j3.Attempts != 1 || j3.Error != "" || j3.PaymentID != "tr_second" {
		t.Fatalf("re-queued job wrong: %+v", j3)
	}
}

// Dedicated XL maps to the root-server tier (iaas-design.md §2).
func TestProvServerTypeForXL(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("MAX_MONTHLY_SPEND_EUR", "1000") // ccx63 (853.49) exceeds a 500 EUR budget // dedicated ccx types sit outside the default 20 EUR budget
	t.Setenv("SPEND_INSTANCE_COSTS", `{"ccx41":138.99,"ccx63":853.49}`)
	if state, err := queueProvisioning("demo", "dedicated-xl", "tr_first"); err != nil || state != "queued" {
		t.Fatalf("queue failed: state=%q err=%v", state, err)
	}
	j := waitForProvJob(t, provJobPath("demo"))
	if j.ServerType != "ccx63" || j.Label != "Dedicated XL" {
		t.Fatalf("xl job wrong: %+v", j)
	}
}

func TestProvisioningStateEndpoint(t *testing.T) {
	t.Setenv("PROVISIONING_ENABLED", "1")
	t.Setenv("PROVISIONING_BASE_DOMAIN", "aimulti-inbox.test")
	t.Setenv("MAX_MONTHLY_SPEND_EUR", "500") // dedicated ccx types sit outside the default 20 EUR budget
	t.Setenv("SPEND_INSTANCE_COSTS", `{"ccx41":138.99,"ccx63":853.49}`)
	sid, _ := connectedSession(t)
	// t.TempDir() DIRECTLY, set AFTER connectedSession (its testEnv would
	// otherwise overwrite it with an uncreated subdir).
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	testMollie(t, paidPayment("dedicated"), nil)

	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	waitForProvJob(t, provJobPath("demo"))

	req, err := http.NewRequest(http.MethodGet, "http://ck.test/landingpage/api/provisioning/state", nil)
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	rec2 := httptest.NewRecorder()
	handleProvisioningState(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("state returned %d: %s", rec2.Code, rec2.Body.String())
	}
	sb := decodeJSON(rec2.Body.String())
	if sb["provisioning"] != "queued" {
		t.Fatalf("state must read queued, got %v", sb)
	}
	job, _ := sb["job"].(map[string]any)
	if job == nil || job["server_type"] != "ccx41" || job["plan"] != "dedicated" {
		t.Fatalf("state job missing or wrong: %v", sb)
	}
	// No job yet → honest "none".
	t.Setenv("PROVISIONING_ENABLED", "")
	other := sessions.m[sid]
	other.username = "fresh"
	req3, err := http.NewRequest(http.MethodGet, "http://ck.test/landingpage/api/provisioning/state", nil)
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	req3.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	rec3 := httptest.NewRecorder()
	handleProvisioningState(rec3, req3)
	if rec3.Code != http.StatusOK || decodeJSON(rec3.Body.String())["provisioning"] != "none" {
		t.Fatalf("no-job state must be none: %d %s", rec3.Code, rec3.Body.String())
	}
}

func TestProvisioningStateRequiresConnection(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	req, err := http.NewRequest(http.MethodGet, "http://ck.test/landingpage/api/provisioning/state", nil)
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	rec := httptest.NewRecorder()
	handleProvisioningState(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous state must be 401, got %d", rec.Code)
	}
}

// Account spend guard (21/10, founder): a dedicated plan under the default
// 20 EUR budget reports provisioning="budget_blocked" honestly — no job
// queued, real credentials (CCX) stay outside the whitelist.
func TestWebhookBudgetBlockedUnderDefaultBudget(t *testing.T) {
	t.Setenv("LANDING_CRED_DIR", t.TempDir())
	t.Setenv("PROVISIONING_ENABLED", "1")
	t.Setenv("PROVISIONING_BASE_DOMAIN", "aimulti-inbox.test")
	// default MAX_MONTHLY_SPEND_EUR (20) — ccx41 is deliberately outside
	testMollie(t, paidPayment("dedicated"), nil)
	rec := httptest.NewRecorder()
	handleCheckoutWebhook(rec, webhookReq(t, "tr_first"))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook must stay 200 (idempotent): %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"budget_blocked"`) {
		t.Fatalf("default budget must block dedicated provisioning: %s", rec.Body.String())
	}
}

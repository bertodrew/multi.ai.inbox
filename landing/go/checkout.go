package main

// checkout.go — self-serve subscriptions via Mollie (decisione founder
// 21/10/2026: Mollie NL — EU data residency, venditori CH ammessi, 1.80%+€0.25
// carte EEA vs Paddle MoR 5%+$0.50; la VAT CH resta del venditore in entrambi
// i casi, quindi conviene il prezzo Mollie).
//
// Flusso: POST api/checkout crea il primo payment (sequenceType=first, che
// fa da mandate SEPA/carta) con metadata {username, plan}; Mollie chiama il
// webhook quando il payment è paid; il webhook verifica il payment
// SERVER-SIDE (mai fidarsi del body), crea la subscription ricorrente e
// persiste lo stato per-account. Fail-closed: senza MOLLIE_API_KEY tutto
// risponde 503 — niente checkout finti.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// checkoutPlan is one self-serve tier, priced in EUR/month (pricing page).
type checkoutPlan struct {
	Price string // Mollie value format: "9.00"
	Label string
}

var checkoutPlans = map[string]checkoutPlan{
	"solo":         {Price: "9.00", Label: "Solo"},
	"studio":       {Price: "25.00", Label: "Studio"},
	"scale":        {Price: "55.00", Label: "Scale"},
	"dedicated":    {Price: "149.00", Label: "Dedicated"},
	"dedicated-xl": {Price: "249.00", Label: "Dedicated XL"},
}

// checkoutPath keys the billing state file to the account (plain JSON — the
// plan and status, not a credential). Same dir + hashing pattern as envPath.
func checkoutPath(username string) string {
	h := sha256.Sum256([]byte(username))
	return fmt.Sprintf("%s/bill_%s.json", env("LANDING_CRED_DIR", "."), hex.EncodeToString(h[:16]))
}

// mollieCall is the Mollie REST client: Bearer key, JSON in/out, raw status.
func mollieCall(verb, path string, body any) (int, map[string]any, error) {
	key := os.Getenv("MOLLIE_API_KEY")
	if key == "" {
		return 0, nil, errPaymentsNotConfigured
	}
	base := strings.TrimRight(env("MOLLIE_API_URL", "https://api.mollie.com/v2"), "/")
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(verb, base+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return res.StatusCode, nil, err
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out, nil
}

type paymentsNotConfiguredError struct{}

func (paymentsNotConfiguredError) Error() string { return "MOLLIE_API_KEY non impostata sul server" }

var errPaymentsNotConfigured = paymentsNotConfiguredError{}

// checkoutBaseURL is the public base for redirectUrl/webhookUrl. The webhook
// must be reachable by Mollie over HTTPS with a valid certificate — set
// LANDING_BASE_URL once the instance has a real domain + cert.
func checkoutBaseURL() string {
	return strings.TrimRight(env("LANDING_BASE_URL", "https://2.28.99.252"), "/")
}

// handleCheckout serves POST api/checkout {plan}: requires a connected
// session (you buy for the connected account) + CSRF. Creates the first
// Mollie payment (sequenceType=first) and returns the hosted checkout URL.
// Fail-closed: no key → 503 (honest — no fake checkout).
func handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	sid, sess := getSession(r)
	if sid == "" || !sess.connected || sess.username == "" {
		jsonErr(w, "Not connected — sign in with your API key first", http.StatusUnauthorized)
		return
	}
	if !requireCSRF(w, r, sess) {
		return
	}
	if os.Getenv("MOLLIE_API_KEY") == "" {
		jsonErr(w, "Payments are not configured yet on this server", http.StatusServiceUnavailable)
		return
	}
	var payload struct {
		Plan string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonErr(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	plan, ok := checkoutPlans[strings.ToLower(strings.TrimSpace(payload.Plan))]
	if !ok {
		jsonErr(w, "Unknown plan", http.StatusNotFound)
		return
	}
	base := checkoutBaseURL()
	body := map[string]any{
		"amount":       map[string]any{"currency": "EUR", "value": plan.Price},
		"description":  fmt.Sprintf("AImulti.inbox %s — monthly", plan.Label),
		"redirectUrl":  base + "/landingpage/settings.html?payment=return",
		"webhookUrl":   base + "/landingpage/api/checkout/webhook",
		"sequenceType": "first",
		"metadata":     map[string]any{"username": sess.username, "plan": strings.ToLower(strings.TrimSpace(payload.Plan))},
	}
	code, out, err := mollieCall(http.MethodPost, "/payments", body)
	if err != nil {
		if err == errPaymentsNotConfigured {
			jsonErr(w, "Payments are not configured yet on this server", http.StatusServiceUnavailable)
		} else {
			jsonErr(w, fmt.Sprintf("payment provider did not answer: %v", err), http.StatusBadGateway)
		}
		return
	}
	if code != http.StatusCreated {
		jsonErr(w, "Payment provider rejected the request", http.StatusBadGateway)
		return
	}
	href := ""
	if links, _ := out["_links"].(map[string]any); links != nil {
		if chk, _ := links["checkout"].(map[string]any); chk != nil {
			href, _ = chk["href"].(string)
		}
	}
	if href == "" {
		jsonErr(w, "Payment provider returned no checkout URL", http.StatusBadGateway)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "checkout_url": href, "plan": payload.Plan})
}

// handleCheckoutWebhook serves POST api/checkout/webhook: Mollie posts
// form-encoded id only — never trust the body; fetch the payment server-side.
// On paid: create the recurring subscription (the first payment is the
// mandate) and persist the state per account. Auth-free by design: Mollie
// calls server-to-server; an id that doesn't fetch under OUR key can't touch
// anything — answer 200 idempotently so Mollie stops retrying.
func handleCheckoutWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if os.Getenv("MOLLIE_API_KEY") == "" {
		jsonErr(w, "Payments are not configured yet on this server", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		jsonErr(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	paymentID := strings.TrimSpace(r.PostFormValue("id"))
	if paymentID == "" {
		jsonErr(w, "Missing payment id", http.StatusBadRequest)
		return
	}
	code, pay, err := mollieCall(http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil)
	if err != nil || code != http.StatusOK {
		// Unknown/unfetchable id — 200 so Mollie stops retrying; no state
		// is touched without a payment verified under our own key.
		jsonOut(w, map[string]any{"ok": true})
		return
	}
	status, _ := pay["status"].(string)
	if status != "paid" {
		jsonOut(w, map[string]any{"ok": true, "status": status})
		return
	}
	meta, _ := pay["metadata"].(map[string]any)
	username, _ := meta["username"].(string)
	planKey, _ := meta["plan"].(string)
	if username == "" {
		jsonOut(w, map[string]any{"ok": true, "status": status, "note": "no metadata"})
		return
	}
	plan, ok := checkoutPlans[planKey]
	if !ok {
		jsonOut(w, map[string]any{"ok": true, "status": status, "note": "unknown plan in metadata"})
		return
	}
	customerID, _ := pay["customerId"].(string)
	subID := ""
	if customerID != "" {
		subBody := map[string]any{
			"amount":      map[string]any{"currency": "EUR", "value": plan.Price},
			"interval":    "1 month",
			"description": fmt.Sprintf("AImulti.inbox %s — monthly", plan.Label),
			"webhookUrl":  checkoutBaseURL() + "/landingpage/api/checkout/webhook",
		}
		scode, sub, _ := mollieCall(http.MethodPost, "/customers/"+url.PathEscape(customerID)+"/subscriptions", subBody)
		if scode == http.StatusCreated && sub != nil {
			subID, _ = sub["id"].(string)
		}
	}
	billBody := fmt.Sprintf(`{"plan":%q,"label":%q,"status":"paid","customer_id":%q,"subscription_id":%q,"first_payment_id":%q,"ts":%q}`,
		planKey, plan.Label, customerID, subID, paymentID, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(checkoutPath(username), []byte(billBody), 0600); err != nil {
		jsonErr(w, "Could not persist billing state", http.StatusInternalServerError)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "status": "paid", "subscription_id": subID})
}

// handleCheckoutState serves GET api/checkout/state: the billing state for
// the connected account (plan, status, subscription id). For the UI.
func handleCheckoutState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	sid, sess := getSession(r)
	if sid == "" || !sess.connected || sess.username == "" {
		jsonErr(w, "Not connected — sign in with your API key first", http.StatusUnauthorized)
		return
	}
	data, err := os.ReadFile(checkoutPath(sess.username))
	if err != nil {
		jsonOut(w, map[string]any{"ok": true, "billing": nil})
		return
	}
	var saved map[string]any
	_ = json.Unmarshal(data, &saved)
	jsonOut(w, map[string]any{"ok": true, "billing": saved})
}

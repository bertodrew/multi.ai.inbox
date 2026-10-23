package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// envMap turns a map into a getenv func, the same shape main passes.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func fullBillingEnv() map[string]string {
	return map[string]string{
		EnvStripeSecretKey:             "sk_test_123",
		EnvStripeWebhookSecret:         "whsec_123",
		PriceEnv("builder", "monthly"): "price_b_m",
		PriceEnv("builder", "yearly"):  "price_b_y",
		PriceEnv("growth", "monthly"):  "price_g_m",
		PriceEnv("growth", "yearly"):   "price_g_y",
		PriceEnv("team", "monthly"):    "price_t_m",
		PriceEnv("team", "yearly"):     "price_t_y",
		EnvBillingReturnURL:            "https://console.example.eu/billing",
	}
}

func TestLoadBillingDisabledWithoutKeys(t *testing.T) {
	b, st := LoadBilling(envMap(map[string]string{}))
	if b != nil {
		t.Fatal("billing must stay nil without keys")
	}
	if st.Enabled {
		t.Fatal("status must report billing off")
	}
	joined := strings.Join(st.Warnings, "; ")
	if !strings.Contains(joined, EnvStripeSecretKey) || !strings.Contains(joined, EnvStripeWebhookSecret) {
		t.Fatalf("warnings must name both variables: %v", st.Warnings)
	}
	if st.WebhookPath != "/stripe/webhook" {
		t.Fatalf("webhook path: %q", st.WebhookPath)
	}
}

// Half a configuration is the mistake worth naming exactly: an operator who
// sets only one key gets 501 billing_disabled and must see which one is missing.
func TestLoadBillingDisabledWithOneKey(t *testing.T) {
	for _, missing := range []string{EnvStripeSecretKey, EnvStripeWebhookSecret} {
		env := fullBillingEnv()
		delete(env, missing)
		b, st := LoadBilling(envMap(env))
		if b != nil || st.Enabled {
			t.Fatalf("billing must be off when %s is missing", missing)
		}
		if !strings.Contains(strings.Join(st.Warnings, "; "), missing) {
			t.Fatalf("warnings must name %s: %v", missing, st.Warnings)
		}
	}
}

func TestLoadBillingEnabledMapsPrices(t *testing.T) {
	b, st := LoadBilling(envMap(fullBillingEnv()))
	if b == nil || !st.Enabled {
		t.Fatalf("billing must be on: %v", st.Warnings)
	}
	if len(b.Prices) != 6 || len(st.Prices) != 6 {
		t.Fatalf("prices: billing %d, status %d", len(b.Prices), len(st.Prices))
	}
	for id, want := range map[string]Price{
		"price_b_m": {"builder", "monthly"},
		"price_g_y": {"growth", "yearly"},
		"price_t_m": {"team", "monthly"},
	} {
		if got := b.Prices[id]; got != want {
			t.Errorf("price %s: got %+v, want %+v", id, got, want)
		}
	}
	if b.ReturnURL != "https://console.example.eu/billing" || st.ReturnURL != b.ReturnURL {
		t.Errorf("return url: %q / %q", b.ReturnURL, st.ReturnURL)
	}
	for _, w := range st.Warnings {
		if strings.Contains(w, "is not set") || strings.Contains(w, "does not look like") {
			t.Errorf("unexpected warning in a complete config: %q", w)
		}
	}
}

// A plan without a price is buyable nowhere; the status must say so instead
// of failing later at Checkout time.
func TestLoadBillingMissingPriceIsReported(t *testing.T) {
	env := fullBillingEnv()
	delete(env, PriceEnv("team", "yearly"))
	_, st := LoadBilling(envMap(env))
	if !strings.Contains(strings.Join(st.Warnings, "; "), PriceEnv("team", "yearly")) {
		t.Fatalf("warnings must name the missing price variable: %v", st.Warnings)
	}
}

func TestLoadBillingDuplicatePriceIsReported(t *testing.T) {
	env := fullBillingEnv()
	env[PriceEnv("team", "yearly")] = "price_b_m" // same id sold as two plans
	_, st := LoadBilling(envMap(env))
	if !strings.Contains(strings.Join(st.Warnings, "; "), "both") {
		t.Fatalf("warnings must report the duplicate price: %v", st.Warnings)
	}
}

func TestLoadBillingFlagsWrongKeyShapeAndBadReturnURL(t *testing.T) {
	env := fullBillingEnv()
	env[EnvStripeSecretKey] = "pk_live_publishable" // the classic mistake
	env[EnvBillingReturnURL] = "console.example.eu/billing"
	_, st := LoadBilling(envMap(env))
	joined := strings.Join(st.Warnings, "; ")
	if !strings.Contains(joined, EnvStripeSecretKey) || !strings.Contains(joined, "sk_") {
		t.Fatalf("publishable key must be flagged: %v", st.Warnings)
	}
	if !strings.Contains(joined, EnvBillingReturnURL) {
		t.Fatalf("relative return url must be flagged: %v", st.Warnings)
	}
	if !st.Enabled {
		t.Fatal("a shape warning must not disable billing on its own")
	}
}

// The status is printed by `mai admin billing-status` and logged at startup,
// so it must never contain key material.
func TestBillingStatusNeverLeaksSecrets(t *testing.T) {
	env := fullBillingEnv()
	env[EnvStripeSecretKey] = "sk_live_supersecretvalue"
	env[EnvStripeWebhookSecret] = "whsec_supersecretvalue"
	b, st := LoadBilling(envMap(env))
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, secret := range []string{env[EnvStripeSecretKey], env[EnvStripeWebhookSecret], "supersecretvalue"} {
		if strings.Contains(out, secret) {
			t.Fatalf("status leaks a Stripe secret: %s", out)
		}
		if b != nil && strings.Contains(b.WebhookSecret, "supersecretvalue") && strings.Contains(out, b.WebhookSecret) {
			t.Fatal("status leaks the webhook secret")
		}
	}
	if !strings.Contains(out, "price_b_m") {
		t.Fatalf("price ids are public and should be reported: %s", out)
	}
}

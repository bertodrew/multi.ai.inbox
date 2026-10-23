package core

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/bertodrew/multi.ai.inbox/internal/stripe"
)

// Billing setting names. The two secrets are read only here; they never
// appear in a status, a log line or an error.
const (
	EnvStripeSecretKey     = "MAI_STRIPE_SECRET_KEY"
	EnvStripeWebhookSecret = "MAI_STRIPE_WEBHOOK_SECRET"
	EnvBillingReturnURL    = "MAI_BILLING_RETURN_URL"
)

// PriceEnv is the variable that carries the Stripe price id of one plan and
// interval, e.g. MAI_STRIPE_PRICE_GROWTH_YEARLY.
func PriceEnv(plan, interval string) string {
	return "MAI_STRIPE_PRICE_" + strings.ToUpper(plan) + "_" + strings.ToUpper(interval)
}

// BillingPrice is one plan/interval that can be bought on this deployment.
// A Stripe price id is public (it appears in Checkout), not a secret.
type BillingPrice struct {
	Plan     string `json:"plan"`
	Interval string `json:"interval"`
	PriceID  string `json:"price_id"`
}

// BillingStatus is the resolved billing configuration, safe to print, log
// and return from a CLI: it says whether billing is on, which prices are
// mapped, and what is missing or suspicious. It never carries key material.
type BillingStatus struct {
	Enabled     bool           `json:"enabled"`
	WebhookPath string         `json:"webhook_path"`
	Prices      []BillingPrice `json:"prices"`
	ReturnURL   string         `json:"return_url,omitempty"`
	// Warnings explain why billing is off, or what is incomplete while it is on.
	Warnings []string `json:"warnings"`
}

// LoadBilling resolves billing from the environment. Billing is on only when
// both MAI_STRIPE_SECRET_KEY and MAI_STRIPE_WEBHOOK_SECRET are non-empty;
// otherwise the returned *Billing is nil and every billing endpoint keeps
// answering 501 billing_disabled. Prices and the return URL are read either
// way so the status can report what is already configured.
//
// getenv is injected (os.Getenv in main) so this is testable offline: no
// network call is made and nothing is validated against Stripe.
func LoadBilling(getenv func(string) string) (*Billing, BillingStatus) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	st := BillingStatus{WebhookPath: "/stripe/webhook", Prices: []BillingPrice{}, Warnings: []string{}}
	key := strings.TrimSpace(getenv(EnvStripeSecretKey))
	secret := strings.TrimSpace(getenv(EnvStripeWebhookSecret))

	prices := map[string]Price{}
	seen := map[string]string{}
	for _, plan := range PaidPlans {
		for _, interval := range BillingIntervals {
			envName := PriceEnv(plan, interval)
			id := strings.TrimSpace(getenv(envName))
			if id == "" {
				st.Warnings = append(st.Warnings,
					fmt.Sprintf("no price for %s %s: %s is not set, so this plan cannot be bought", plan, interval, envName))
				continue
			}
			if other, dup := seen[id]; dup {
				st.Warnings = append(st.Warnings,
					fmt.Sprintf("price %s is configured for both %s and %s %s", id, other, plan, interval))
			}
			seen[id] = plan + " " + interval
			prices[id] = Price{Plan: plan, Interval: interval}
			st.Prices = append(st.Prices, BillingPrice{Plan: plan, Interval: interval, PriceID: id})
		}
	}

	if ret := strings.TrimSpace(getenv(EnvBillingReturnURL)); ret != "" {
		st.ReturnURL = ret
		if u, err := url.Parse(ret); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			st.Warnings = append(st.Warnings,
				fmt.Sprintf("%s must be an absolute http or https URL; callers must then pass success_url and cancel_url", EnvBillingReturnURL))
		}
	}

	switch {
	case key == "" && secret == "":
		st.Warnings = append(st.Warnings, EnvStripeSecretKey+" and "+EnvStripeWebhookSecret+" are not set")
		return nil, st
	case key == "":
		st.Warnings = append(st.Warnings, EnvStripeSecretKey+" is not set")
		return nil, st
	case secret == "":
		st.Warnings = append(st.Warnings, EnvStripeWebhookSecret+" is not set")
		return nil, st
	}

	// Shape checks catch the common operator mistake (a publishable key or a
	// truncated paste) without calling Stripe. Test keys sk_test_* pass.
	if !strings.HasPrefix(key, "sk_") && !strings.HasPrefix(key, "rk_") {
		st.Warnings = append(st.Warnings, EnvStripeSecretKey+" does not look like a Stripe secret key (sk_ or rk_)")
	}
	if !strings.HasPrefix(secret, "whsec_") {
		st.Warnings = append(st.Warnings, EnvStripeWebhookSecret+" does not look like a Stripe endpoint secret (whsec_)")
	}

	st.Enabled = true
	return &Billing{
		Stripe:        &stripe.Client{SecretKey: key},
		WebhookSecret: secret,
		Prices:        prices,
		ReturnURL:     st.ReturnURL,
	}, st
}

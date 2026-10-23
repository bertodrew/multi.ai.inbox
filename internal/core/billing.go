package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/stripe"
)

// BillingGracePeriod is how long a past_due or unpaid subscription keeps its
// paid plan before the scheduler moves the organization to free.
const BillingGracePeriod = 7 * 24 * time.Hour

// PaidPlans can be bought through Stripe Checkout, cheapest first.
var PaidPlans = []string{"builder", "growth", "team"}

// BillingIntervals are the Stripe price intervals we sell (yearly = 10 × monthly, doc 11).
var BillingIntervals = []string{"monthly", "yearly"}

// Price tells which plan and interval a Stripe price id stands for.
type Price struct {
	Plan     string
	Interval string
}

// Billing is the Stripe configuration. A nil Service.Billing disables every
// billing endpoint with 501 billing_disabled.
type Billing struct {
	Stripe        *stripe.Client
	WebhookSecret string
	// Prices maps Stripe price ids to plans (MAI_STRIPE_PRICE_<PLAN>_<INTERVAL>).
	Prices map[string]Price
	// ReturnURL is the default success, cancel and portal return URL.
	ReturnURL string
}

func (b *Billing) priceID(plan, interval string) string {
	for id, p := range b.Prices {
		if p.Plan == plan && p.Interval == interval {
			return id
		}
	}
	return ""
}

func (s *Service) billing() (*Billing, error) {
	if s.Billing == nil {
		return nil, errf(http.StatusNotImplemented, "billing_disabled", "billing is not configured on this deployment")
	}
	return s.Billing, nil
}

func providerError(err error) error {
	var api *stripe.APIError
	if errors.As(err, &api) {
		return errf(http.StatusBadGateway, "billing_provider_error", "the payment provider rejected the request (%s %s)", api.Type, api.Code)
	}
	return errf(http.StatusBadGateway, "billing_provider_error", "the payment provider is unreachable, retry later")
}

// BillingInfo is the billing state of an organization (GET /v0/billing).
type BillingInfo struct {
	Plan              string     `json:"plan"`
	Status            string     `json:"status"`
	Interval          string     `json:"interval,omitempty"`
	CurrentPeriodEnd  *time.Time `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool       `json:"cancel_at_period_end"`
	PastDueSince      *time.Time `json:"past_due_since,omitempty"`
	GraceEndsAt       *time.Time `json:"grace_period_ends_at,omitempty"`
}

type subscriptionRow struct {
	customer, subscription, status, priceID, interval *string
	periodEnd, pastDueSince, lastEventAt              *time.Time
	cancelAtPeriodEnd                                 bool
}

const subscriptionCols = `stripe_customer_id, stripe_subscription_id, status, price_id, billing_interval,
	current_period_end, past_due_since, last_event_at, cancel_at_period_end`

func scanSubscription(row pgx.Row, r *subscriptionRow) error {
	return row.Scan(&r.customer, &r.subscription, &r.status, &r.priceID, &r.interval,
		&r.periodEnd, &r.pastDueSince, &r.lastEventAt, &r.cancelAtPeriodEnd)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// live statuses hold (or are about to hold) a paid plan.
func liveStatus(status string) bool {
	return slices.Contains([]string{"active", "trialing", "past_due", "unpaid"}, status)
}

// orgSubscription reads the subscription row of orgID in a tenant transaction.
func (s *Service) orgSubscription(ctx context.Context, orgID string) (*Org, *subscriptionRow, error) {
	o := &Org{}
	var sub *subscriptionRow
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		if err := scanOrg(tx.QueryRow(ctx, "select "+orgCols+" from organizations where id = $1", orgID), o); err != nil {
			return err
		}
		r := &subscriptionRow{}
		err := scanSubscription(tx.QueryRow(ctx, "select "+subscriptionCols+" from subscriptions where org_id = $1", orgID), r)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		sub = r
		return err
	})
	return o, sub, err
}

// GetBilling returns the plan and the Stripe subscription state.
func (s *Service) GetBilling(ctx context.Context, orgID string) (*BillingInfo, error) {
	if _, err := s.billing(); err != nil {
		return nil, err
	}
	o, sub, err := s.orgSubscription(ctx, orgID)
	if err != nil {
		return nil, err
	}
	info := &BillingInfo{Plan: o.Plan, Status: "none"}
	if sub != nil {
		info.Status = derefStr(sub.status)
		info.Interval = derefStr(sub.interval)
		info.CurrentPeriodEnd = sub.periodEnd
		info.CancelAtPeriodEnd = sub.cancelAtPeriodEnd
		info.PastDueSince = sub.pastDueSince
		if sub.pastDueSince != nil && (info.Status == "past_due" || info.Status == "unpaid") {
			end := sub.pastDueSince.Add(BillingGracePeriod)
			info.GraceEndsAt = &end
		}
	}
	return info, nil
}

type CheckoutRequest struct {
	Plan       string `json:"plan"`
	Interval   string `json:"interval"`
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
}

func returnURL(field, v, def string) (string, error) {
	if v == "" {
		v = def
	}
	if v == "" {
		return "", Validation("%s is required (no MAI_BILLING_RETURN_URL default is configured)", field)
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(v) > 2000 {
		return "", Validation("%s must be an absolute http or https URL", field)
	}
	return v, nil
}

// CreateCheckout starts a Stripe Checkout Session in subscription mode for a
// paid plan. Stripe Tax computes EU VAT (automatic_tax) and the customer can
// enter a VAT ID for the reverse charge (tax_id_collection); both need Stripe
// Tax to be set up in the Stripe dashboard.
func (s *Service) CreateCheckout(ctx context.Context, orgID string, req CheckoutRequest) (*stripe.Session, error) {
	b, err := s.billing()
	if err != nil {
		return nil, err
	}
	if !slices.Contains(PaidPlans, req.Plan) {
		return nil, Validation("plan must be one of builder, growth, team")
	}
	if req.Interval == "" {
		req.Interval = "monthly"
	}
	if !slices.Contains(BillingIntervals, req.Interval) {
		return nil, Validation("interval must be monthly or yearly")
	}
	price := b.priceID(req.Plan, req.Interval)
	if price == "" {
		return nil, Validation("the %s %s price is not configured on this deployment", req.Plan, req.Interval)
	}
	success, err := returnURL("success_url", req.SuccessURL, b.ReturnURL)
	if err != nil {
		return nil, err
	}
	cancel, err := returnURL("cancel_url", req.CancelURL, b.ReturnURL)
	if err != nil {
		return nil, err
	}
	_, sub, err := s.orgSubscription(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if sub != nil && sub.subscription != nil && liveStatus(derefStr(sub.status)) {
		return nil, errf(http.StatusConflict, "already_subscribed",
			"this organization already has a subscription; change plan or interval in the customer portal (POST /v0/billing/portal)")
	}
	form := url.Values{
		"mode":                                {"subscription"},
		"line_items[0][price]":                {price},
		"line_items[0][quantity]":             {"1"},
		"success_url":                         {success},
		"cancel_url":                          {cancel},
		"client_reference_id":                 {orgID},
		"metadata[org_id]":                    {orgID},
		"subscription_data[metadata][org_id]": {orgID},
		"automatic_tax[enabled]":              {"true"},
		"tax_id_collection[enabled]":          {"true"},
		"billing_address_collection":          {"required"},
	}
	if sub != nil && sub.customer != nil {
		form.Set("customer", *sub.customer)
		// Tax and VAT ID collection on an existing customer must be allowed to update it.
		form.Set("customer_update[address]", "auto")
		form.Set("customer_update[name]", "auto")
	}
	session, err := b.Stripe.CreateCheckoutSession(ctx, form)
	if err != nil {
		return nil, providerError(err)
	}
	return session, nil
}

// CreatePortal opens the Stripe Customer Portal (plan changes, payment
// method, invoices, cancellation) for an organization that has a customer.
func (s *Service) CreatePortal(ctx context.Context, orgID, returnTo string) (*stripe.Session, error) {
	b, err := s.billing()
	if err != nil {
		return nil, err
	}
	ret, err := returnURL("return_url", returnTo, b.ReturnURL)
	if err != nil {
		return nil, err
	}
	_, sub, err := s.orgSubscription(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.customer == nil {
		return nil, errf(http.StatusConflict, "no_billing_account", "this organization has no Stripe customer yet; start with POST /v0/billing/checkout")
	}
	session, err := b.Stripe.CreatePortalSession(ctx, *sub.customer, ret)
	if err != nil {
		return nil, providerError(err)
	}
	return session, nil
}

// Stripe webhook outcomes.
const (
	StripeApplied   = "applied"
	StripeDuplicate = "duplicate"
	StripeIgnored   = "ignored"
	StripeStale     = "stale"
)

// billingChange is what an event says about one subscription.
type billingChange struct {
	orgHint      string // metadata org_id or client_reference_id
	customer     string
	subscription string
	paymentFail  bool // invoice.payment_failed: only the status changes
	status       string
	priceID      string
	periodEnd    *time.Time
	cancelAtEnd  bool
}

func subscriptionChange(sub *stripe.Subscription) *billingChange {
	return &billingChange{orgHint: sub.Metadata["org_id"], customer: sub.Customer, subscription: sub.ID,
		status: sub.Status, priceID: sub.PriceID(), periodEnd: sub.PeriodEnd(), cancelAtEnd: sub.CancelAtPeriodEnd}
}

// change decodes an event; nil means the event type is not used. It may call
// the Stripe API (checkout completion reads the subscription it created).
func (s *Service) change(ctx context.Context, b *Billing, ev *stripe.Event) (*billingChange, error) {
	switch ev.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(ev.Data.Object, &cs); err != nil {
			return nil, Validation("invalid checkout session: %v", err)
		}
		if cs.Mode != "subscription" || cs.Subscription == "" {
			return nil, nil
		}
		sub, err := b.Stripe.GetSubscription(ctx, cs.Subscription)
		if err != nil {
			return nil, providerError(err)
		}
		ch := subscriptionChange(sub)
		if ch.customer == "" {
			ch.customer = cs.Customer
		}
		if cs.ClientReferenceID != "" {
			ch.orgHint = cs.ClientReferenceID
		} else if v := cs.Metadata["org_id"]; v != "" {
			ch.orgHint = v
		}
		return ch, nil
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(ev.Data.Object, &sub); err != nil {
			return nil, Validation("invalid subscription: %v", err)
		}
		ch := subscriptionChange(&sub)
		if ev.Type == "customer.subscription.deleted" {
			ch.status = "canceled"
		}
		return ch, nil
	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(ev.Data.Object, &inv); err != nil {
			return nil, Validation("invalid invoice: %v", err)
		}
		if inv.SubscriptionID() == "" {
			return nil, nil // one off invoice, no plan attached
		}
		return &billingChange{orgHint: inv.Parent.SubscriptionDetails.Metadata["org_id"], customer: inv.Customer,
			subscription: inv.SubscriptionID(), paymentFail: true, status: "past_due"}, nil
	}
	return nil, nil
}

// HandleStripeWebhook verifies and applies one Stripe event. The event id
// is recorded in the same transaction as its effect, so redeliveries are no
// ops; an error leaves nothing recorded and Stripe retries.
func (s *Service) HandleStripeWebhook(ctx context.Context, payload []byte, signature string) (string, error) {
	b, err := s.billing()
	if err != nil {
		return "", err
	}
	if err := stripe.VerifySignature(payload, signature, b.WebhookSecret, s.now(), stripe.DefaultTolerance); err != nil {
		return "", errf(http.StatusBadRequest, "invalid_signature", "Stripe-Signature verification failed")
	}
	ev := &stripe.Event{}
	if err := json.Unmarshal(payload, ev); err != nil || ev.ID == "" || ev.Type == "" {
		return "", Validation("invalid Stripe event")
	}
	var seen bool
	if err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select exists(select 1 from stripe_events where id = $1)", ev.ID).Scan(&seen)
	}); err != nil {
		return "", err
	}
	if seen {
		return StripeDuplicate, nil
	}
	ch, err := s.change(ctx, b, ev)
	if err != nil {
		return "", err
	}
	outcome := StripeIgnored
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "insert into stripe_events (id, type) values ($1, $2) on conflict (id) do nothing", ev.ID, ev.Type)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			outcome = StripeDuplicate // a concurrent delivery won
			return nil
		}
		if ch == nil {
			return nil
		}
		orgID, err := resolveBillingOrg(ctx, tx, ch)
		if err != nil || orgID == "" {
			return err
		}
		if _, err := tx.Exec(ctx, "update stripe_events set org_id = $2 where id = $1", ev.ID, orgID); err != nil {
			return err
		}
		outcome, err = s.applyChange(ctx, tx, b, orgID, ev, ch)
		return err
	})
	if err != nil {
		return "", err
	}
	return outcome, nil
}

// resolveBillingOrg finds the organization of a change: the org id we put in
// the Checkout metadata, else the stored customer or subscription id.
func resolveBillingOrg(ctx context.Context, tx pgx.Tx, ch *billingChange) (string, error) {
	var id string
	if ch.orgHint != "" {
		err := tx.QueryRow(ctx, "select id from organizations where id::text = $1", ch.orgHint).Scan(&id)
		if err == nil || !errors.Is(err, pgx.ErrNoRows) {
			return id, err
		}
	}
	err := tx.QueryRow(ctx, `select org_id from subscriptions
		where (stripe_customer_id = $1 and $1 <> '') or (stripe_subscription_id = $2 and $2 <> '') limit 1`,
		ch.customer, ch.subscription).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Service) applyChange(ctx context.Context, tx pgx.Tx, b *Billing, orgID string, ev *stripe.Event, ch *billingChange) (string, error) {
	if _, err := tx.Exec(ctx, "insert into subscriptions (org_id) values ($1) on conflict (org_id) do nothing", orgID); err != nil {
		return "", err
	}
	cur := &subscriptionRow{}
	if err := scanSubscription(tx.QueryRow(ctx, "select "+subscriptionCols+" from subscriptions where org_id = $1 for update", orgID), cur); err != nil {
		return "", err
	}
	var plan string
	if err := tx.QueryRow(ctx, "select plan from organizations where id = $1 for update", orgID).Scan(&plan); err != nil {
		return "", err
	}
	created := time.Unix(ev.Created, 0).UTC()
	if cur.lastEventAt != nil && created.Before(*cur.lastEventAt) {
		return StripeStale, nil
	}
	// An event about a replaced subscription (cancelled, then bought again)
	// must not touch the live one.
	if old := derefStr(cur.subscription); old != "" && ch.subscription != "" && old != ch.subscription &&
		liveStatus(derefStr(cur.status)) && !liveStatus(ch.status) {
		return StripeIgnored, nil
	}

	now := s.now()
	next := *cur
	next.status = &ch.status
	if ch.customer != "" {
		next.customer = &ch.customer
	}
	if ch.subscription != "" {
		next.subscription = &ch.subscription
	}
	newPlan := plan
	if ch.paymentFail {
		if derefStr(cur.status) == "canceled" {
			next.status = cur.status
		}
	} else {
		next.periodEnd = ch.periodEnd
		next.cancelAtPeriodEnd = ch.cancelAtEnd
		if ch.priceID != "" {
			next.priceID = &ch.priceID
			if p, ok := b.Prices[ch.priceID]; ok {
				next.interval = &p.Interval
			}
		}
	}
	switch derefStr(next.status) {
	case "active", "trialing":
		next.pastDueSince = nil
		if p, ok := b.Prices[ch.priceID]; ok {
			newPlan = p.Plan
		}
	case "past_due", "unpaid":
		// Grace period: keep the plan, start the clock once.
		if cur.pastDueSince == nil {
			next.pastDueSince = &now
		}
	case "canceled", "incomplete_expired", "paused":
		next.pastDueSince = nil
		newPlan = "free"
	}
	if _, err := tx.Exec(ctx, `update subscriptions set stripe_customer_id = $2, stripe_subscription_id = $3, status = $4,
		price_id = $5, billing_interval = $6, current_period_end = $7, cancel_at_period_end = $8, past_due_since = $9,
		last_event_at = $10, updated_at = now() where org_id = $1`,
		orgID, next.customer, next.subscription, derefStr(next.status), next.priceID, next.interval, next.periodEnd,
		next.cancelAtPeriodEnd, next.pastDueSince, created); err != nil {
		return "", err
	}
	if newPlan != plan {
		if _, err := tx.Exec(ctx, "update organizations set plan = $2 where id = $1", orgID, newPlan); err != nil {
			return "", err
		}
	}
	meta, _ := json.Marshal(map[string]any{"event_id": ev.ID, "event_type": ev.Type, "status": derefStr(next.status),
		"plan_from": plan, "plan_to": newPlan, "subscription": nullIfEmpty(derefStr(next.subscription))})
	if _, err := tx.Exec(ctx, `insert into audit_log (org_id, actor, action, target, metadata) values ($1, 'stripe', 'billing.update', $2, $3)`,
		orgID, orgID, meta); err != nil {
		return "", err
	}
	return StripeApplied, nil
}

// DowngradeLapsedSubscriptions moves organizations whose subscription has
// been past_due or unpaid for longer than the grace period to the free plan.
// The subscription row keeps its status; a later successful payment
// (customer.subscription.updated, active) restores the paid plan.
func (s *Service) DowngradeLapsedSubscriptions(ctx context.Context) (int, error) {
	cutoff := s.now().Add(-BillingGracePeriod)
	n := 0
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with lapsed as (
				select o.id, o.plan from subscriptions s join organizations o on o.id = s.org_id
				where s.status in ('past_due', 'unpaid') and s.past_due_since <= $1 and o.plan <> 'free'
				for update of o)
			update organizations o set plan = 'free' from lapsed where o.id = lapsed.id
			returning o.id, lapsed.plan`, cutoff)
		if err != nil {
			return err
		}
		type lapsed struct{ id, plan string }
		var list []lapsed
		for rows.Next() {
			var l lapsed
			if err := rows.Scan(&l.id, &l.plan); err != nil {
				rows.Close()
				return err
			}
			list = append(list, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, l := range list {
			meta, _ := json.Marshal(map[string]any{"plan_from": l.plan, "plan_to": "free", "reason": "payment past due beyond the grace period"})
			if _, err := tx.Exec(ctx, `insert into audit_log (org_id, actor, action, target, metadata)
				values ($1, 'system:billing', 'billing.downgrade', $2, $3)`, l.id, l.id, meta); err != nil {
				return err
			}
		}
		n = len(list)
		return nil
	})
	return n, err
}

// Package stripe is a minimal Stripe client written against the REST API
// with net/http and form encoding, without the Stripe SDK: Checkout and
// Customer Portal sessions, subscription lookup and webhook signature
// verification. It holds no business rules; those live in core.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Stripe API. Tests point BaseURL at a fake server.
const DefaultBaseURL = "https://api.stripe.com"

// Client calls the Stripe API with a secret key. The key is sent only in
// the Authorization header and never appears in errors or logs.
type Client struct {
	SecretKey string
	BaseURL   string
	HTTP      *http.Client
}

// APIError is a non 2xx answer from Stripe. Error() carries the HTTP status,
// type and code only: Stripe messages can echo request data.
type APIError struct {
	Status  int
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("stripe: HTTP %d %s %s", e.Status, e.Type, e.Code)
}

func (c *Client) do(ctx context.Context, method, path string, form url.Values, out any) error {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(base, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return fmt.Errorf("stripe: %s %s: %w", method, path, uerr.Err)
		}
		return fmt.Errorf("stripe: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("stripe: read response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		var env struct {
			Error APIError `json:"error"`
		}
		_ = json.Unmarshal(raw, &env)
		env.Error.Status = resp.StatusCode
		return &env.Error
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("stripe: decode %s: %w", path, err)
	}
	return nil
}

// Session is a Checkout or Customer Portal session; URL is where to send the user.
type Session struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// CreateCheckoutSession posts form to /v1/checkout/sessions.
func (c *Client) CreateCheckoutSession(ctx context.Context, form url.Values) (*Session, error) {
	s := &Session{}
	return s, c.do(ctx, http.MethodPost, "/v1/checkout/sessions", form, s)
}

// CreatePortalSession opens the Customer Portal for a customer.
func (c *Client) CreatePortalSession(ctx context.Context, customer, returnURL string) (*Session, error) {
	s := &Session{}
	return s, c.do(ctx, http.MethodPost, "/v1/billing_portal/sessions", url.Values{"customer": {customer}, "return_url": {returnURL}}, s)
}

// GetSubscription reads the current state of a subscription.
func (c *Client) GetSubscription(ctx context.Context, id string) (*Subscription, error) {
	s := &Subscription{}
	return s, c.do(ctx, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(id), nil, s)
}

// Event is a webhook event; Data.Object depends on Type.
type Event struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// CheckoutSession is the object of checkout.session.completed.
type CheckoutSession struct {
	ID                string            `json:"id"`
	Mode              string            `json:"mode"`
	ClientReferenceID string            `json:"client_reference_id"`
	Customer          string            `json:"customer"`
	Subscription      string            `json:"subscription"`
	Metadata          map[string]string `json:"metadata"`
}

// Subscription is the object of customer.subscription.* events. Since API
// version 2025-03-31 the period lives on the items; both shapes are read.
type Subscription struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Status            string            `json:"status"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	CurrentPeriodEnd  int64             `json:"current_period_end"`
	Metadata          map[string]string `json:"metadata"`
	Items             struct {
		Data []struct {
			CurrentPeriodEnd int64 `json:"current_period_end"`
			Price            struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

// PriceID returns the price of the first item (one plan per subscription).
func (s *Subscription) PriceID() string {
	if len(s.Items.Data) == 0 {
		return ""
	}
	return s.Items.Data[0].Price.ID
}

// PeriodEnd returns the end of the current billing period, if known.
func (s *Subscription) PeriodEnd() *time.Time {
	end := s.CurrentPeriodEnd
	if end == 0 && len(s.Items.Data) > 0 {
		end = s.Items.Data[0].CurrentPeriodEnd
	}
	if end == 0 {
		return nil
	}
	t := time.Unix(end, 0).UTC()
	return &t
}

// Invoice is the object of invoice.payment_failed. Since API version
// 2025-03-31 the subscription moved under parent.subscription_details.
type Invoice struct {
	ID           string `json:"id"`
	Customer     string `json:"customer"`
	Subscription string `json:"subscription"`
	Parent       struct {
		SubscriptionDetails struct {
			Subscription string            `json:"subscription"`
			Metadata     map[string]string `json:"metadata"`
		} `json:"subscription_details"`
	} `json:"parent"`
}

// SubscriptionID returns the invoice's subscription in either API shape.
func (i *Invoice) SubscriptionID() string {
	if i.Subscription != "" {
		return i.Subscription
	}
	return i.Parent.SubscriptionDetails.Subscription
}

// DefaultTolerance is how old a signed webhook may be (Stripe's default).
const DefaultTolerance = 5 * time.Minute

var (
	ErrNoSignature  = errors.New("stripe: missing or malformed Stripe-Signature header")
	ErrBadSignature = errors.New("stripe: no v1 signature matches the payload")
	ErrTooOld       = errors.New("stripe: signature timestamp outside the tolerance")
)

// Sign returns a Stripe-Signature header value for payload, as Stripe
// computes it. Used by tests and local tooling.
func Sign(payload []byte, secret string, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	return "t=" + ts + ",v1=" + signature(payload, secret, ts)
}

func signature(payload []byte, secret, ts string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks a Stripe-Signature header ("t=...,v1=...,v1=...")
// against the raw request body: HMAC SHA256 of "t.payload" with the
// endpoint secret, compared in constant time with every v1 entry, and a
// timestamp no further than tolerance from now in either direction.
func VerifySignature(payload []byte, header, secret string, now time.Time, tolerance time.Duration) error {
	if secret == "" {
		return ErrBadSignature
	}
	var ts string
	var sigs [][]byte
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			if b, err := hex.DecodeString(v); err == nil {
				sigs = append(sigs, b)
			}
		}
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return ErrNoSignature
	}
	expected, _ := hex.DecodeString(signature(payload, secret, ts))
	match := false
	for _, s := range sigs {
		if hmac.Equal(expected, s) {
			match = true
		}
	}
	if !match {
		return ErrBadSignature
	}
	if d := now.Sub(time.Unix(unix, 0)); d > tolerance || d < -tolerance {
		return ErrTooOld
	}
	return nil
}

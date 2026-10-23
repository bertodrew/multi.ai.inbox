package stripe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const secret = "whsec_test_secret"

func TestVerifySignature(t *testing.T) {
	payload := []byte(`{"id":"evt_1","type":"invoice.payment_failed"}`)
	now := time.Unix(1_800_000_000, 0)
	valid := Sign(payload, secret, now)

	cases := []struct {
		name   string
		header string
		secret string
		now    time.Time
		want   error
	}{
		{"valid", valid, secret, now, nil},
		{"valid within tolerance", valid, secret, now.Add(4 * time.Minute), nil},
		{"one of several v1 matches", "t=1800000000,v1=" + strings.Repeat("ab", 32) + "," + strings.TrimPrefix(valid, "t=1800000000,"), secret, now, nil},
		{"wrong secret", valid, "whsec_other", now, ErrBadSignature},
		{"empty secret", valid, "", now, ErrBadSignature},
		{"old timestamp", valid, secret, now.Add(6 * time.Minute), ErrTooOld},
		{"future timestamp", valid, secret, now.Add(-6 * time.Minute), ErrTooOld},
		{"tampered timestamp", strings.Replace(valid, "t=1800000000", "t=1800000001", 1), secret, now, ErrBadSignature},
		{"missing header", "", secret, now, ErrNoSignature},
		{"no v1", "t=1800000000,v0=abc", secret, now, ErrNoSignature},
		{"garbage", "nonsense", secret, now, ErrNoSignature},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := VerifySignature(payload, c.header, c.secret, c.now, DefaultTolerance)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
	if err := VerifySignature([]byte(`{"id":"evt_2"}`), valid, secret, now, DefaultTolerance); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered payload: %v", err)
	}
}

func TestClientFormAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk_test_123" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"api_key_invalid","message":"Invalid API Key provided: sk_test_***"}}`))
			return
		}
		_ = r.ParseForm()
		if r.URL.Path != "/v1/checkout/sessions" || r.PostForm.Get("line_items[0][price]") != "price_1" ||
			r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"id":"cs_1","url":"https://checkout.stripe.test/cs_1"}`))
	}))
	defer srv.Close()

	c := &Client{SecretKey: "sk_test_123", BaseURL: srv.URL}
	s, err := c.CreateCheckoutSession(context.Background(), map[string][]string{"line_items[0][price]": {"price_1"}})
	if err != nil || s.URL != "https://checkout.stripe.test/cs_1" {
		t.Fatalf("checkout: %v %+v", err, s)
	}
	bad := &Client{SecretKey: "sk_test_wrong", BaseURL: srv.URL}
	_, err = bad.CreateCheckoutSession(context.Background(), nil)
	var api *APIError
	if !errors.As(err, &api) || api.Status != 401 || api.Code != "api_key_invalid" {
		t.Fatalf("want a 401 APIError, got %v", err)
	}
	if strings.Contains(err.Error(), "sk_test") {
		t.Fatalf("error text must not carry key material: %q", err.Error())
	}
}

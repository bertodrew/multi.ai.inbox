// Package aimulti is a client for the multi.ai.inbox REST API, email inboxes
// for AI agents.
//
// The client speaks /v1, the current version of the API. Every method takes a
// context first, returns typed values that mirror openapi/openapi.yaml, and
// returns a *Error carrying the API's error code and the request id when the
// API answers with a 4xx or 5xx.
//
//	c := aimulti.NewClient(os.Getenv("MAI_API_KEY"))
//	me, err := c.Me(ctx)
//
// The package has no dependencies outside the standard library.
package aimulti

import (
	"bytes"
	"context"
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

// DefaultBaseURL is the production API. Deployments that are not on the public
// address (staging, self hosted) pass WithBaseURL.
const DefaultBaseURL = "https://api.aimulti.inbox"

// Version is the API version the client speaks by default.
const Version = "v1"

// maxResponse bounds a response body; a raw message may be up to 25 MB.
const maxResponse = 64 << 20

// Client calls the REST API with one API key.
type Client struct {
	apiKey    string
	baseURL   string
	version   string
	userAgent string
	http      *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another deployment, for example
// "https://api.staging.example.eu". A trailing slash is ignored.
func WithBaseURL(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.baseURL = strings.TrimSuffix(base, "/")
		}
	}
}

// WithHTTPClient replaces the HTTP client, for a proxy, a custom timeout or a
// traceable transport. The zero value is used when nil is passed.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithAPIVersion speaks an older version. Pass "v0" only for a deployment that
// predates /v1: /v0 is deprecated and its responses carry Sunset and Link
// headers naming /v1 as the successor. An unrecognised value leaves the
// default in place.
func WithAPIVersion(v string) Option {
	return func(c *Client) {
		switch v {
		case "v0", "v1":
			c.version = v
		}
	}
}

// NewClient returns a client for the API key. An empty key is not refused here:
// the API answers 401 missing_authorization with a clear message, which is a
// better error than a panic at construction.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:    apiKey,
		baseURL:   DefaultBaseURL,
		version:   Version,
		userAgent: "aimulti-go/" + Version,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Error is a failed API call: the HTTP status, the error envelope the API
// returned (name, code, message) and the request id to quote to support.
type Error struct {
	Status     int
	Name       string
	Code       string
	Message    string
	RequestID  string
	RetryAfter int // seconds, when the API sent Retry-After
	Method     string
	URL        string
}

func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "aimulti: %s %s: %d", e.Method, e.URL, e.Status)
	if e.Code != "" {
		fmt.Fprintf(&b, " %s", e.Code)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request %s)", e.RequestID)
	}
	return b.String()
}

// IsCode reports whether err is an API error with this code, for example
// "already_exists". A transport error never matches.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// StatusCode returns the HTTP status of an API error, or 0.
func StatusCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// RetryAfter returns how long the API asked the caller to wait, or 0.
func RetryAfter(err error) time.Duration {
	var e *Error
	if errors.As(err, &e) && e.RetryAfter > 0 {
		return time.Duration(e.RetryAfter) * time.Second
	}
	return 0
}

// callOptions are per call settings that travel in headers, never in a body.
type callOptions struct {
	idempotencyKey string
}

// CallOption adjusts a single call.
type CallOption func(*callOptions)

// WithIdempotencyKey makes a send safe to retry: a repeated key returns the
// first result instead of a second message. Up to 256 characters, remembered
// for 24 hours.
func WithIdempotencyKey(key string) CallOption {
	return func(o *callOptions) { o.idempotencyKey = key }
}

type request struct {
	method string
	path   string // relative to /v1, for example "/inboxes"
	query  url.Values
	body   any
	opts   []CallOption
	accept string
}

// call performs one HTTP request and returns the body, the headers and the
// status. A 4xx or 5xx is turned into a *Error.
func (c *Client) call(ctx context.Context, r request) ([]byte, http.Header, error) {
	u := c.baseURL + "/" + c.version + r.path
	if len(r.query) > 0 {
		u += "?" + r.query.Encode()
	}
	var body io.Reader
	if r.body != nil {
		raw, err := json.Marshal(r.body)
		if err != nil {
			return nil, nil, fmt.Errorf("aimulti: encode %s %s: %w", r.method, r.path, err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, u, body)
	if err != nil {
		return nil, nil, fmt.Errorf("aimulti: build %s %s: %w", r.method, r.path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if r.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.accept != "" {
		req.Header.Set("Accept", r.accept)
	}
	o := callOptions{}
	for _, opt := range r.opts {
		opt(&o)
	}
	if o.idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", o.idempotencyKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("aimulti: %s %s: %w", r.method, r.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if readErr != nil {
		return nil, resp.Header, fmt.Errorf("aimulti: read %s %s: %w", r.method, r.path, readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, resp.Header, apiError(resp, raw, r.method, u)
	}
	return raw, resp.Header, nil
}

// apiError builds a *Error from an error response, falling back to the status
// line when the body is not the JSON envelope.
func apiError(resp *http.Response, raw []byte, method, u string) *Error {
	e := &Error{
		Status:    resp.StatusCode,
		Message:   http.StatusText(resp.StatusCode),
		RequestID: resp.Header.Get("X-Request-Id"),
		Method:    method,
		URL:       u,
	}
	var envelope struct {
		Name    string `json:"name"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Code != "" {
		e.Name = envelope.Name
		e.Code = envelope.Code
		e.Message = envelope.Message
	}
	if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		e.RetryAfter = n
	}
	return e
}

// getJSON performs a GET and decodes the body into out.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.decode(ctx, request{method: http.MethodGet, path: path, query: query}, out)
}

// sendJSON performs a method with a JSON body and decodes the response.
func (c *Client) sendJSON(ctx context.Context, method, path string, body any, opts []CallOption, out any) error {
	return c.decode(ctx, request{method: method, path: path, body: body, opts: opts}, out)
}

func (c *Client) decode(ctx context.Context, r request, out any) error {
	raw, _, err := c.call(ctx, r)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("aimulti: decode %s %s: %w", r.method, r.path, err)
	}
	return nil
}

// escape encodes one path segment, for an address, a Message-ID or an id.
func escape(s string) string { return url.PathEscape(s) }

// itoa and join keep the option helpers free of fmt.
func itoa(n int) string { return strconv.Itoa(n) }

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func join(items []string, sep string) string { return strings.Join(items, sep) }

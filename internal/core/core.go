// Package core holds the business logic shared by every surface: REST API,
// MCP server, CLI, SMTP edge and workers. One capability, one implementation.
package core

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/db"
)

type Service struct {
	DB                *db.DB
	Blob              blob.Store
	SharedDomain      string // free inboxes live here, e.g. agents.example.eu
	Hostname          string // used in generated Message-IDs and HELO
	AllowHTTPWebhooks bool   // development only
	SPFInclude        string // relay SPF include, e.g. spf.brevo.com
	DNS               Resolver
	SendMaxAge        time.Duration // retry window for temporary send failures; 0 = DefaultSendMaxAge
	Now               func() time.Time
	Billing           *Billing     // Stripe; nil disables billing (501 billing_disabled)
	Log               *slog.Logger // nil = slog.Default()
	// Antivirus scans API uploaded attachments and agent to agent mail
	// (MAI_CLAMD_ADDR). When it cannot scan, content is accepted with the
	// av-unscanned label, or refused when AntivirusFailClosed is set.
	Antivirus           Scanner
	AntivirusFailClosed bool
}

// Plan holds the limits of a pricing plan (doc 01, section 7). The 5 minute
// burst is 10% of the daily cap (doc 11, section 5.2).
type Plan struct {
	Name          string `json:"name"`
	Inboxes       int    `json:"inboxes"`
	Domains       int    `json:"domains"`
	SendsPerDay   int    `json:"sends_per_day"`
	SendsPerMonth int    `json:"sends_per_month"`
	SendsPer5Min  int    `json:"sends_per_5_minutes"`
	Footer        bool   `json:"footer"`
}

var Plans = map[string]Plan{
	"free":    {Name: "free", Inboxes: 3, Domains: 1, SendsPerDay: 100, SendsPerMonth: 3000, SendsPer5Min: 10, Footer: true},
	"builder": {Name: "builder", Inboxes: 10, Domains: 5, SendsPerDay: 1000, SendsPerMonth: 10000, SendsPer5Min: 100},
	"growth":  {Name: "growth", Inboxes: 50, Domains: 20, SendsPerDay: 2500, SendsPerMonth: 40000, SendsPer5Min: 250},
	"team":    {Name: "team", Inboxes: 150, Domains: 50, SendsPerDay: 15000, SendsPerMonth: 150000, SendsPer5Min: 1500},
}

func planOf(name string) Plan {
	if p, ok := Plans[name]; ok {
		return p
	}
	return Plans["free"]
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Error is the API error envelope: {name, code, message}.
type Error struct {
	Status  int    `json:"-"`
	Name    string `json:"name"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// RetryAfter, in seconds, sets the Retry-After header of a 429.
	RetryAfter int `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func errf(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Name: http.StatusText(status), Code: code, Message: fmt.Sprintf(format, args...)}
}

func Validation(format string, args ...any) *Error {
	return errf(http.StatusBadRequest, "validation_error", format, args...)
}
func NotFound(what string) *Error {
	return errf(http.StatusNotFound, "not_found", "%s not found", what)
}

// AsError converts any error into the envelope; unknown errors become internal_error.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return errf(http.StatusInternalServerError, "internal_error", "internal error")
}

// Page is a cursor page request. Token encodes the last (timestamp, id) seen.
type Page struct {
	Limit int
	Token string
}

func (p Page) limit() int {
	switch {
	case p.Limit <= 0:
		return 20
	case p.Limit > 100:
		return 100
	}
	return p.Limit
}

type cursor struct {
	At time.Time
	ID string
}

func (p Page) cursor() (*cursor, error) {
	if p.Token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(p.Token)
	if err != nil {
		return nil, Validation("invalid page_token")
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, Validation("invalid page_token")
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return nil, Validation("invalid page_token")
	}
	return &cursor{At: at, ID: id}, nil
}

func nextToken(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// System labels are set by the platform and cannot be changed through the API.
var systemLabels = map[string]bool{"received": true, "sent": true, "queued": true, "failed": true, "bounced": true, "complained": true}

func uniqueLabels(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, l := range in {
		l = strings.TrimSpace(strings.ToLower(l))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

func validateCustomLabels(labels []string) error {
	for _, l := range labels {
		if systemLabels[strings.ToLower(l)] {
			return Validation("label %q is managed by the platform", l)
		}
		if len(l) > 64 {
			return Validation("label %q is longer than 64 characters", l)
		}
	}
	return nil
}

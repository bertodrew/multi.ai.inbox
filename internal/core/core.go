// Package core holds the business logic shared by every surface: REST API,
// MCP server, CLI, SMTP edge and workers. One capability, one implementation.
package core

import (
	"encoding/base64"
	"errors"
	"fmt"
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
	Now               func() time.Time
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
var systemLabels = map[string]bool{"received": true, "sent": true, "queued": true, "failed": true}

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

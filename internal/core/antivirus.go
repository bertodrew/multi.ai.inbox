package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/clamav"
	"github.com/bertodrew/multi.ai.inbox/internal/mailx"
)

// Scanner scans content for malware; *clamav.Clamd implements it.
type Scanner interface {
	Scan(ctx context.Context, data []byte) (clamav.Result, error)
}

// ErrInfected marks agent to agent mail that the virus scanner flagged. It is
// a permanent failure (never retried) but not a hard bounce: the recipient
// address is fine, the content is not.
var ErrInfected = fmt.Errorf("%w: virus detected", ErrPermanent)

// AVAlertInterval is the minimum time between two security.av_unavailable
// events for the same organization from one process.
const AVAlertInterval = 10 * time.Minute

// avUnscanned counts content accepted without a virus scan by this process.
var avUnscanned atomic.Int64

// avAlerts holds the last security.av_unavailable event per organization,
// for the whole process.
var avAlerts struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// AVUnscannedTotal returns how many times this process accepted content
// without a virus scan (the av-unscanned label) since it started.
func AVUnscannedTotal() int64 { return avUnscanned.Load() }

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// NoteAVUnscanned is the alerting hook for content accepted without a scan
// because clamd could not answer (fail open). source is mx, internal (agent
// to agent) or api. It logs at warn with the running process counter, which
// is what log based alerting keys on.
func (s *Service) NoteAVUnscanned(source string, err error) {
	n := avUnscanned.Add(1)
	s.log().Warn("antivirus unavailable, content accepted without virus scan",
		"source", source, "err", err, "av_unscanned_total", n)
}

// avAlertDue reports whether an organization may get another
// security.av_unavailable event, and if so starts its quiet period.
func (s *Service) avAlertDue(orgID string) bool {
	now := s.now()
	avAlerts.mu.Lock()
	defer avAlerts.mu.Unlock()
	if last, ok := avAlerts.last[orgID]; ok && now.Sub(last) < AVAlertInterval && now.Sub(last) >= 0 {
		return false
	}
	if avAlerts.last == nil {
		avAlerts.last = map[string]time.Time{}
	}
	avAlerts.last[orgID] = now
	return true
}

// alertAVUnscanned emits security.av_unavailable for the organization that
// received unscanned content, at most once per AVAlertInterval per process.
func (s *Service) alertAVUnscanned(ctx context.Context, tx pgx.Tx, orgID, inboxID, address, source string) error {
	if !s.avAlertDue(orgID) {
		return nil
	}
	return insertEvent(ctx, tx, orgID, inboxID, "security.av_unavailable", map[string]any{
		"inbox_id": address,
		"source":   source,
		"reason":   "the virus scanner was unavailable; content was accepted with the av-unscanned label",
		"interval": AVAlertInterval.String(),
	})
}

// scanAttachments scans attachments uploaded through the API before the send
// is queued. An infected attachment fails the request with 422
// attachment_infected. When clamd cannot scan, the request fails with 503
// antivirus_unavailable if AntivirusFailClosed is set; otherwise it goes on
// and unscanned is true (the message gets the av-unscanned label).
func (s *Service) scanAttachments(ctx context.Context, atts []mailx.Attachment) (unscanned bool, err error) {
	if s.Antivirus == nil {
		return false, nil
	}
	for _, a := range atts {
		res, err := s.Antivirus.Scan(ctx, a.Data)
		switch {
		case err != nil && s.AntivirusFailClosed:
			s.log().Error("clamd unavailable, refusing attachments", "source", "api", "err", err)
			return false, errf(http.StatusServiceUnavailable, "antivirus_unavailable", "the virus scanner is unavailable; retry later")
		case err != nil:
			s.NoteAVUnscanned("api", err)
			return true, nil
		case res.Infected:
			return false, errf(http.StatusUnprocessableEntity, "attachment_infected",
				"attachment %q was rejected: virus detected (%s)", a.Filename, res.Signature)
		}
	}
	return false, nil
}

// ScanInternal scans agent to agent mail before local delivery and returns
// the antivirus result for authentication_results: clean, unscanned (fail
// open) or empty when no scanner is configured. Infected mail returns
// ErrInfected; with AntivirusFailClosed an unavailable scanner returns a
// temporary error, so the sender retries.
func (s *Service) ScanInternal(ctx context.Context, raw []byte) (string, error) {
	if s.Antivirus == nil {
		return "", nil
	}
	res, err := s.Antivirus.Scan(ctx, raw)
	switch {
	case err != nil && s.AntivirusFailClosed:
		return "", fmt.Errorf("virus scanner unavailable: %w", err)
	case err != nil:
		s.NoteAVUnscanned("internal", err)
		return "unscanned", nil
	case res.Infected:
		return "", fmt.Errorf("%w (%s)", ErrInfected, res.Signature)
	}
	return "clean", nil
}

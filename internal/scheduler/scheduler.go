// Package scheduler runs periodic platform jobs: domain verification, the
// screening timeout, scheduled drafts, blob deletion for deleted inboxes, the
// address quarantine purge and the billing grace period (retention and warm up
// later).
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

type Scheduler struct {
	Svc      *core.Service
	Log      *slog.Logger
	Interval time.Duration
}

func (s *Scheduler) Run(ctx context.Context) {
	interval := s.Interval
	if interval == 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := s.Svc.VerifyPendingDomains(ctx); err != nil {
			s.Log.Error("domain verification failed", "err", err)
		} else if n > 0 {
			s.Log.Info("domains checked", "count", n)
		}
		// Fail closed: mail still unscreened after the timeout is held for review.
		if n, err := s.Svc.ExpireScreening(ctx, core.DefaultScreeningTimeout); err != nil {
			s.Log.Error("screening timeout sweep failed", "err", err)
		} else if n > 0 {
			s.Log.Warn("screening timed out, messages held for review", "count", n)
		}
		s.sendDrafts(ctx)
		s.purgeBlobs(ctx)
		// Retention promise of the privacy policy: audit entries older than
		// core.AuditLogRetention are dropped, so the published 90 days is
		// enforced and not a target.
		if n, err := s.Svc.PurgeExpiredAudit(ctx, core.AuditLogRetention); err != nil {
			s.Log.Error("audit retention sweep failed", "err", err)
		} else if n > 0 {
			s.Log.Info("expired audit entries purged", "count", n)
		}
		// Push jobs are a queue, not a log: finished rows go after a week.
		if n, err := s.Svc.PurgePushJobs(ctx); err != nil {
			s.Log.Error("push job retention sweep failed", "err", err)
		} else if n > 0 {
			s.Log.Info("finished push jobs purged", "count", n)
		}
		if n, err := s.Svc.PurgeExpiredQuarantine(ctx); err != nil {
			s.Log.Error("quarantine purge failed", "err", err)
		} else if n > 0 {
			s.Log.Info("expired address quarantines purged", "count", n)
		}
		if n, err := s.Svc.DowngradeLapsedSubscriptions(ctx); err != nil {
			s.Log.Error("billing grace period sweep failed", "err", err)
		} else if n > 0 {
			s.Log.Warn("organizations moved to free after the billing grace period", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// purgeBlobs drains the blob deletion queue in batches.
func (s *Scheduler) purgeBlobs(ctx context.Context) {
	total := 0
	for ctx.Err() == nil {
		n, err := s.Svc.ProcessBlobDeletions(ctx, 200)
		total += n
		if err != nil {
			s.Log.Error("blob deletion failed", "err", err)
		}
		if n < 200 {
			break // queue drained, or the rest is waiting for a retry
		}
	}
	if total > 0 {
		s.Log.Info("blobs deleted", "count", total)
	}
}

// sendDrafts sends the scheduled drafts that are due, in batches.
func (s *Scheduler) sendDrafts(ctx context.Context) {
	for ctx.Err() == nil {
		sent, failed, err := s.Svc.SendDueDrafts(ctx, 100)
		if err != nil {
			s.Log.Error("scheduled draft send failed", "err", err)
		}
		if sent > 0 || failed > 0 {
			s.Log.Info("scheduled drafts processed", "sent", sent, "refused", failed)
		}
		if sent+failed < 100 || err != nil {
			return
		}
	}
}

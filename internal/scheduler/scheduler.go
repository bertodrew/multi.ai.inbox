// Package scheduler runs periodic platform jobs: domain verification, the
// screening timeout, blob deletion for deleted inboxes and the address
// quarantine purge (retention and warm up later).
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
		s.purgeBlobs(ctx)
		if n, err := s.Svc.PurgeExpiredQuarantine(ctx); err != nil {
			s.Log.Error("quarantine purge failed", "err", err)
		} else if n > 0 {
			s.Log.Info("expired address quarantines purged", "count", n)
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

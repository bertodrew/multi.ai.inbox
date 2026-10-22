// Package scheduler runs periodic platform jobs (domain verification and the
// screening timeout today; retention and warm up later).
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
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Package scheduler runs periodic platform jobs (domain verification today;
// retention and warm up later).
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
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

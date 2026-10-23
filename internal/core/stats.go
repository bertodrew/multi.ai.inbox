package core

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// QueueStats are the backlogs exported as gauges on /metrics.
type QueueStats struct {
	OutboxPending    int64   // events not yet fanned out to webhooks
	OutboxLagSeconds float64 // age of the oldest of them, 0 when none
	ScreeningPending int64   // inbound messages waiting for a gatekeeper verdict
	ScreeningReview  int64   // messages held for review
	BlobDeletions    int64   // blobs queued for deletion
}

// QueueStats reads the backlogs across all organizations. Each query uses a
// partial or small index (events_pending, messages_held, blob_deletions_due),
// so it is cheap enough for every scrape.
func (s *Service) QueueStats(ctx context.Context) (QueueStats, error) {
	var st QueueStats
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select
			(select count(*) from events where fanned_out_at is null),
			coalesce((select extract(epoch from now() - created_at)::float8 from events
				where fanned_out_at is null order by id limit 1), 0),
			(select count(*) from messages where screening = 'pending'),
			(select count(*) from messages where screening = 'review'),
			(select count(*) from blob_deletions)`).
			Scan(&st.OutboxPending, &st.OutboxLagSeconds, &st.ScreeningPending, &st.ScreeningReview, &st.BlobDeletions)
	})
	return st, err
}

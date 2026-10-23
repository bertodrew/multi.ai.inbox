package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AuditLogRetention is how long security relevant audit entries are kept.
// It is the value the privacy policy publishes; the scheduler enforces it
// with PurgeExpiredAudit, so the promise is not just a target.
const AuditLogRetention = 90 * 24 * time.Hour

// PurgeExpiredAudit deletes audit entries older than retention and returns
// how many it removed. Entries are dropped across tenants (system
// transaction) because retention is a platform-wide obligation, not a
// per-organization setting.
func (s *Service) PurgeExpiredAudit(ctx context.Context, retention time.Duration) (int64, error) {
	if retention <= 0 {
		retention = AuditLogRetention
	}
	var n int64
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "delete from audit_log where created_at < now() - $1::interval", retention.String())
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}

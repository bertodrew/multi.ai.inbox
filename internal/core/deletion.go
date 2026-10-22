package core

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// AddressQuarantine is how long a deleted address stays reserved. The
// database trigger on inboxes (migration 0006) applies it; this constant is
// the documented value.
const AddressQuarantine = 90 * 24 * time.Hour

// blobDeleteLease covers one deletion attempt; a worker that crashes mid
// batch leaves rows that become due again when the lease expires.
const blobDeleteLease = 5 * time.Minute

// quarantinedUntil returns the end of the quarantine of address, if any.
// It runs inside the caller's transaction, tenant or system.
func quarantinedUntil(ctx context.Context, tx pgx.Tx, address string) (time.Time, bool, error) {
	var until time.Time
	err := tx.QueryRow(ctx, `select quarantined_until from address_quarantine
		where address = $1 and quarantined_until > now() order by quarantined_until desc limit 1`, address).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return until, err == nil, err
}

func quarantineError(address string, until time.Time) *Error {
	return errf(http.StatusConflict, "address_quarantined",
		"address %s belonged to a deleted inbox and cannot be reused until %s", address, until.UTC().Format(time.RFC3339))
}

type blobDeletion struct {
	id  int64
	key string
}

// ProcessBlobDeletions removes up to n queued blobs (raw .eml files and
// attachments of deleted messages) and returns how many it removed. Rows are
// leased first, so concurrent schedulers never work on the same key, and a
// row is deleted only after its blob is gone.
func (s *Service) ProcessBlobDeletions(ctx context.Context, n int) (int, error) {
	var batch []blobDeletion
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `update blob_deletions set attempts = attempts + 1, next_attempt_at = now() + $2::interval
			where id in (select id from blob_deletions where next_attempt_at <= now()
				order by next_attempt_at limit $1 for update skip locked)
			returning id, storage_key`, n, blobDeleteLease.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b blobDeletion
			if err := rows.Scan(&b.id, &b.key); err != nil {
				return err
			}
			batch = append(batch, b)
		}
		return rows.Err()
	})
	if err != nil || len(batch) == 0 {
		return 0, err
	}
	var done []int64
	var failures []error
	for _, b := range batch {
		if delErr := s.Blob.Delete(ctx, b.key); delErr != nil {
			failures = append(failures, delErr)
			// Keep the row; retry after a backoff that grows with the attempts.
			_ = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `update blob_deletions set last_error = $2,
					next_attempt_at = now() + least(attempts, 60) * interval '1 minute' where id = $1`, b.id, delErr.Error())
				return err
			})
			continue
		}
		done = append(done, b.id)
	}
	if len(done) > 0 {
		if err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "delete from blob_deletions where id = any($1)", done)
			return err
		}); err != nil {
			return 0, err
		}
	}
	return len(done), errors.Join(failures...)
}

// PurgeExpiredQuarantine drops quarantine rows whose window has ended.
func (s *Service) PurgeExpiredQuarantine(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "delete from address_quarantine where quarantined_until <= now()")
		n = tag.RowsAffected()
		return err
	})
	return n, err
}

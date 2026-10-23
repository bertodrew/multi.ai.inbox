package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// OrgDeletionSummary is the final record of an organization, exported before
// it is deleted. The audit log is stored per organization and goes with it,
// so the operator keeps this summary (with the full audit log) instead.
type OrgDeletionSummary struct {
	Organization   *Org           `json:"organization"`
	CreatedAt      time.Time      `json:"created_at"`
	ExportedAt     time.Time      `json:"exported_at"`
	Inboxes        []string       `json:"inboxes"`        // addresses, quarantined for 90 days by the delete
	Domains        []string       `json:"custom_domains"` // released by the delete
	Counts         map[string]int `json:"counts"`
	APIKeysRevoked int            `json:"api_keys_revoked"`
	Audit          []*AuditEntry  `json:"audit_log"`
}

// OrgDeletionResult reports what the delete did.
type OrgDeletionResult struct {
	OrganizationID       string   `json:"organization_id"`
	Deleted              bool     `json:"deleted"`
	BlobsQueued          int      `json:"blobs_queued"`
	AddressesQuarantined int      `json:"addresses_quarantined"`
	DomainsReleased      []string `json:"custom_domains_released"`
}

// DeleteOrg deletes an organization and everything it owns (operator
// command). confirm must equal the organization's name. In order:
//
//  1. every API key is revoked and an org.delete audit entry is written, in
//     one committed transaction, so no request can act for the organization
//     while the rest runs;
//  2. the summary, with the full audit log, is handed to export; when export
//     fails nothing is deleted (run the command again);
//  3. the organization row is deleted and the cascade removes inboxes,
//     threads, messages, attachments, domains, webhooks, events, keys and the
//     audit log. The triggers of migration 0006 queue every blob for the
//     scheduler and quarantine every address for 90 days in the same
//     transaction, and custom domain names become free to add again.
func (s *Service) DeleteOrg(ctx context.Context, orgID, confirm string, export func(*OrgDeletionSummary) error) (*OrgDeletionResult, error) {
	org := &Org{}
	var created time.Time
	revoked := 0
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "select "+orgCols+", created_at from organizations where id::text = $1 for update", orgID).
			Scan(&org.ID, &org.Name, &org.Plan, &org.ScreenBeforeNotify, &created)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("organization")
		}
		if err != nil {
			return err
		}
		if confirm != org.Name {
			return Validation("confirmation %q does not match the organization name %q", confirm, org.Name)
		}
		tag, err := tx.Exec(ctx, "update api_keys set revoked_at = now() where org_id = $1 and revoked_at is null", org.ID)
		if err != nil {
			return err
		}
		revoked = int(tag.RowsAffected())
		meta, _ := json.Marshal(map[string]any{"name": org.Name, "api_keys_revoked": revoked})
		_, err = tx.Exec(ctx, `insert into audit_log (org_id, actor, action, target, metadata) values ($1, 'operator', 'org.delete', $2, $3)`,
			org.ID, org.ID, meta)
		return err
	})
	if err != nil {
		return nil, err
	}

	sum := &OrgDeletionSummary{Organization: org, CreatedAt: created, ExportedAt: s.now().UTC(), APIKeysRevoked: revoked,
		Inboxes: []string{}, Domains: []string{}, Counts: map[string]int{}, Audit: []*AuditEntry{}}
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var err error
		if sum.Inboxes, err = queryStrings(ctx, tx, "select address::text from inboxes where org_id = $1 order by address", org.ID); err != nil {
			return err
		}
		if sum.Domains, err = queryStrings(ctx, tx, "select name::text from domains where org_id = $1 order by name", org.ID); err != nil {
			return err
		}
		for _, t := range []string{"api_keys", "threads", "messages", "attachments", "webhooks", "events", "send_outcomes"} {
			var n int
			if err := tx.QueryRow(ctx, fmt.Sprintf("select count(*) from %s where org_id = $1", t), org.ID).Scan(&n); err != nil {
				return err
			}
			sum.Counts[t] = n
		}
		rows, err := tx.Query(ctx, `select id, actor, action, target, ip, request_id, metadata, created_at
			from audit_log where org_id = $1 order by id`, org.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e := &AuditEntry{}
			if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.IP, &e.RequestID, &e.Metadata, &e.CreatedAt); err != nil {
				return err
			}
			sum.Audit = append(sum.Audit, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if err := export(sum); err != nil {
		return nil, fmt.Errorf("export summary (nothing deleted; API keys stay revoked): %w", err)
	}

	res := &OrgDeletionResult{OrganizationID: org.ID, Deleted: true, DomainsReleased: sum.Domains}
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var before, quarantined int
		if err := tx.QueryRow(ctx, "select count(*) from blob_deletions where org_id = $1", org.ID).Scan(&before); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "select count(*) from address_quarantine where org_id = $1", org.ID).Scan(&quarantined); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "delete from organizations where id = $1", org.ID); err != nil {
			return err
		}
		var after, nowQuarantined int
		if err := tx.QueryRow(ctx, "select count(*) from blob_deletions where org_id = $1", org.ID).Scan(&after); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "select count(*) from address_quarantine where org_id = $1", org.ID).Scan(&nowQuarantined); err != nil {
			return err
		}
		res.BlobsQueued, res.AddressesQuarantined = after-before, nowQuarantined-quarantined
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func queryStrings(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[string])
	return append([]string{}, list...), err
}

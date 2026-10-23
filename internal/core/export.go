package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ExportFormat identifies the shape of an organization export. Bump the
// suffix when fields are removed or change meaning; adding fields keeps it.
const ExportFormat = "aimulti.inbox/export@1"

// ExportNotice explains what the export deliberately leaves out. It travels
// inside the file so a data subject reading it does not have to guess.
const ExportNotice = "Account export: organization settings, inboxes, custom domains, " +
	"webhooks, API key metadata and the security audit log. Mailbox contents " +
	"(message bodies and attachments) are not duplicated here: the customer can " +
	"export them in full over IMAP, and a single message over " +
	"GET /v0/inboxes/{inbox}/messages/{message}/raw. Secrets (API key material, " +
	"webhook signing secrets, DKIM private keys, password hashes) are never " +
	"exported. Timestamps are RFC 3339 in UTC."

// ExportInbox is one mailbox in the export, without message contents.
type ExportInbox struct {
	Address     string    `json:"address"`
	DisplayName string    `json:"display_name"`
	ClientID    *string   `json:"client_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Messages    int       `json:"messages"`
}

// ExportDomain is one domain owned by the organization.
type ExportDomain struct {
	Name       string     `json:"domain"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

// ExportWebhook is one webhook; the signing secret is not included.
type ExportWebhook struct {
	ID         string    `json:"webhook_id"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// ExportAPIKey is key metadata only: never the key, never the hash.
type ExportAPIKey struct {
	ID          string     `json:"api_key_id"`
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// OrgExport is the answer to GET /v0/export: everything the platform holds
// about the organization as a tenant, in one JSON document.
type OrgExport struct {
	Format       string          `json:"format"`
	GeneratedAt  time.Time       `json:"generated_at"`
	Organization *Org            `json:"organization"`
	Inboxes      []ExportInbox   `json:"inboxes"`
	Domains      []ExportDomain  `json:"domains"`
	Webhooks     []ExportWebhook `json:"webhooks"`
	APIKeys      []ExportAPIKey  `json:"api_keys"`
	Counts       map[string]int  `json:"counts"`
	AuditLog     []*AuditEntry   `json:"audit_log"`
	Notice       string          `json:"notice"`
}

// ExportOrg reads the organization's own data for a portability request
// (Art. 15 and 20 GDPR). It runs inside the tenant transaction, so row level
// security applies and one organization can never read another.
func (s *Service) ExportOrg(ctx context.Context, p *Principal) (*OrgExport, error) {
	if err := p.Require(PermDataExport); err != nil {
		return nil, err
	}
	orgID := p.Org.ID
	out := &OrgExport{
		Format: ExportFormat, GeneratedAt: s.now().UTC(), Organization: &Org{},
		Inboxes: []ExportInbox{}, Domains: []ExportDomain{}, Webhooks: []ExportWebhook{},
		APIKeys: []ExportAPIKey{}, Counts: map[string]int{}, AuditLog: []*AuditEntry{},
		Notice: ExportNotice,
	}
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		if err := scanOrg(tx.QueryRow(ctx, "select "+orgCols+" from organizations where id = $1", orgID), out.Organization); err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `select i.address::text, i.display_name, i.client_id, i.created_at,
				(select count(*) from messages m where m.inbox_id = i.id)
			from inboxes i where i.org_id = $1 order by i.address`, orgID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var in ExportInbox
			if err := rows.Scan(&in.Address, &in.DisplayName, &in.ClientID, &in.CreatedAt, &in.Messages); err != nil {
				rows.Close()
				return err
			}
			out.Inboxes = append(out.Inboxes, in)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		rows, err = tx.Query(ctx, `select name::text, case when verified_at is null then 'pending' else 'verified' end,
				created_at, verified_at
			from domains where org_id = $1 order by name`, orgID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d ExportDomain
			if err := rows.Scan(&d.Name, &d.Status, &d.CreatedAt, &d.VerifiedAt); err != nil {
				rows.Close()
				return err
			}
			out.Domains = append(out.Domains, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		rows, err = tx.Query(ctx, `select id::text, url, event_types, enabled, created_at
			from webhooks where org_id = $1 order by created_at`, orgID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var w ExportWebhook
			if err := rows.Scan(&w.ID, &w.URL, &w.EventTypes, &w.Enabled, &w.CreatedAt); err != nil {
				rows.Close()
				return err
			}
			out.Webhooks = append(out.Webhooks, w)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		rows, err = tx.Query(ctx, `select id::text, name, permissions, created_at, expires_at, last_used_at, revoked_at
			from api_keys where org_id = $1 order by created_at`, orgID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k ExportAPIKey
			if err := rows.Scan(&k.ID, &k.Name, &k.Permissions, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
				rows.Close()
				return err
			}
			out.APIKeys = append(out.APIKeys, k)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, t := range []string{"threads", "messages", "attachments", "drafts", "events", "webhooks", "api_keys"} {
			var n int
			if err := tx.QueryRow(ctx, "select count(*) from "+t+" where org_id = $1", orgID).Scan(&n); err != nil {
				return err
			}
			out.Counts[t] = n
		}

		audit, err := tx.Query(ctx, `select id, actor, action, target, ip, request_id, metadata, created_at
			from audit_log where org_id = $1 order by id`, orgID)
		if err != nil {
			return err
		}
		defer audit.Close()
		for audit.Next() {
			e := &AuditEntry{}
			if err := audit.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.IP, &e.RequestID, &e.Metadata, &e.CreatedAt); err != nil {
				return err
			}
			out.AuditLog = append(out.AuditLog, e)
		}
		return audit.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

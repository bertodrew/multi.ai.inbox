package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/keys"
)

// Permissions an API key can hold. A key without a permission list holds all
// of them (operator keys and keys created before scopes existed).
const (
	PermInboxRead     = "inbox_read"
	PermInboxWrite    = "inbox_write"
	PermMessageRead   = "message_read"
	PermMessageSend   = "message_send"
	PermMessageUpdate = "message_update"
	PermDomainRead    = "domain_read"
	PermDomainWrite   = "domain_write"
	PermWebhookRead   = "webhook_read"
	PermWebhookWrite  = "webhook_write"
	PermAPIKeyRead    = "api_key_read"
	PermAPIKeyWrite   = "api_key_write"
	PermAuditRead     = "audit_read"
	// PermMessageReview lets a key read messages held for review
	// (include_review=true). Give it to a human review console, not to agents.
	PermMessageReview = "message_review"
	// PermMessageRelease lets a key release a held message, which then fires
	// message.received. Same advice: never to the agent that reads the inbox.
	PermMessageRelease = "message_release"
	// PermBillingRead reads the plan and subscription state; PermBillingWrite
	// starts a Stripe Checkout or opens the customer portal.
	PermBillingRead  = "billing_read"
	PermBillingWrite = "billing_write"
)

var AllPermissions = []string{PermInboxRead, PermInboxWrite, PermMessageRead, PermMessageSend, PermMessageUpdate,
	PermDomainRead, PermDomainWrite, PermWebhookRead, PermWebhookWrite, PermAPIKeyRead, PermAPIKeyWrite, PermAuditRead,
	PermMessageReview, PermMessageRelease, PermBillingRead, PermBillingWrite}

// ExplicitPermissions are never implied by a full access key (nil
// permissions): a key holds them only when they are listed, so an agent with a
// full key cannot approve the prompt injection it was screened from. Only the
// operator (mai admin key-create -permissions) can issue them first.
var ExplicitPermissions = []string{PermMessageRelease}

// Principal is the authenticated caller of a request.
type Principal struct {
	Org         *Org
	KeyID       string
	Permissions []string // nil means all
	ExpiresAt   *time.Time
}

func (p *Principal) Can(perm string) bool {
	if p.Permissions == nil {
		return !slices.Contains(ExplicitPermissions, perm)
	}
	return slices.Contains(p.Permissions, perm)
}

// Require returns a 403 missing_permission error when p lacks perm.
func (p *Principal) Require(perm string) error {
	if p.Can(perm) {
		return nil
	}
	return errf(http.StatusForbidden, "missing_permission", "this API key lacks the %s permission", perm)
}

// ReviewAccess checks an include_review request: asking for held messages
// needs message_review on top of message_read.
func (p *Principal) ReviewAccess(includeReview bool) error {
	if !includeReview {
		return nil
	}
	return p.Require(PermMessageReview)
}

func (p *Principal) Actor() string { return "api_key:" + p.KeyID }

// Authenticate resolves an API key to its organization and permissions.
func (s *Service) Authenticate(ctx context.Context, plain string) (*Principal, error) {
	unknown := errf(http.StatusUnauthorized, "unknown_api_key", "invalid API key")
	if !keys.WellFormed(plain) {
		return nil, unknown
	}
	p := &Principal{Org: &Org{}}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `update api_keys k set last_used_at = now()
			from organizations o
			where k.hash = $1 and k.revoked_at is null and o.id = k.org_id
			returning k.id, k.permissions, k.expires_at, o.id, o.name, o.plan, o.screen_before_notify`, keys.Hash(plain)).
			Scan(&p.KeyID, &p.Permissions, &p.ExpiresAt, &p.Org.ID, &p.Org.Name, &p.Org.Plan, &p.Org.ScreenBeforeNotify)
		if errors.Is(err, pgx.ErrNoRows) {
			return unknown
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if p.ExpiresAt != nil && s.now().After(*p.ExpiresAt) {
		return nil, errf(http.StatusUnauthorized, "api_key_expired", "this API key expired at %s", p.ExpiresAt.Format(time.RFC3339))
	}
	return p, nil
}

type APIKey struct {
	ID          string     `json:"api_key_id"`
	Key         string     `json:"api_key,omitempty"` // only on create
	Prefix      string     `json:"prefix"`
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"` // null means all
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreatedKey is kept for the operator commands' output shape.
type CreatedKey = APIKey

// CreateKey is the operator path: a key with every permission.
func (s *Service) CreateKey(ctx context.Context, orgID, name string) (*APIKey, error) {
	return s.insertKey(ctx, orgID, name, nil, nil, "")
}

// CreateOperatorKey is the operator path for a key with a fixed permission
// list, the only way to issue the first key holding an explicit permission
// such as message_release.
func (s *Service) CreateOperatorKey(ctx context.Context, orgID, name string, perms []string) (*APIKey, error) {
	if len(perms) == 0 {
		return nil, Validation("permissions must not be empty")
	}
	for _, perm := range perms {
		if !slices.Contains(AllPermissions, perm) {
			return nil, Validation("unknown permission %q; valid: %s", perm, strings.Join(AllPermissions, ", "))
		}
	}
	perms = append([]string{}, slices.Compact(slices.Sorted(slices.Values(perms)))...)
	return s.insertKey(ctx, orgID, name, perms, nil, "")
}

type CreateKeyRequest struct {
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// CreateScopedKey lets a key create another key with at most its own permissions.
func (s *Service) CreateScopedKey(ctx context.Context, p *Principal, req CreateKeyRequest) (*APIKey, error) {
	if err := p.Require(PermAPIKeyWrite); err != nil {
		return nil, err
	}
	if len(req.Name) > 100 {
		return nil, Validation("name is longer than 100 characters")
	}
	perms := req.Permissions
	if perms == nil {
		perms = p.Permissions // inherit: a full key creates a full key, a scoped key a copy of its scope
	}
	for _, perm := range perms {
		if !slices.Contains(AllPermissions, perm) {
			return nil, Validation("unknown permission %q; valid: %s", perm, strings.Join(AllPermissions, ", "))
		}
		if !p.Can(perm) {
			return nil, errf(http.StatusForbidden, "permission_escalation", "cannot grant %s: the calling key does not hold it", perm)
		}
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(s.now()) {
		return nil, Validation("expires_at must be in the future")
	}
	if p.ExpiresAt != nil && (req.ExpiresAt == nil || req.ExpiresAt.After(*p.ExpiresAt)) {
		req.ExpiresAt = p.ExpiresAt // a child never outlives its parent
	}
	if perms != nil {
		if len(perms) == 0 {
			return nil, Validation("permissions must not be empty; omit the field to copy the calling key's permissions")
		}
		// append onto a non nil slice: a nil result would be stored as NULL, which means every permission.
		perms = append([]string{}, slices.Compact(slices.Sorted(slices.Values(perms)))...)
	}
	return s.insertKey(ctx, p.Org.ID, req.Name, perms, req.ExpiresAt, p.KeyID)
}

func (s *Service) insertKey(ctx context.Context, orgID, name string, perms []string, expires *time.Time, parent string) (*APIKey, error) {
	plain, prefix, hash, err := keys.New()
	if err != nil {
		return nil, err
	}
	if parent != "" && perms == nil && !s.parentIsFull(ctx, orgID, parent) {
		// Fail closed: a child of a scoped key must never become a full access key.
		return nil, errf(http.StatusForbidden, "permission_escalation", "a scoped key cannot create a full access key")
	}
	k := &APIKey{Key: plain, Prefix: prefix, Name: name, Permissions: perms, ExpiresAt: expires}
	var createdBy any
	if parent != "" {
		createdBy = parent
	}
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into api_keys (org_id, name, prefix, hash, permissions, expires_at, created_by)
			values ($1, $2, $3, $4, $5, $6, $7) returning id, created_at`,
			orgID, name, prefix, hash, perms, expires, createdBy).Scan(&k.ID, &k.CreatedAt)
	})
	return k, err
}

func (s *Service) ListKeys(ctx context.Context, p *Principal) ([]*APIKey, error) {
	if err := p.Require(PermAPIKeyRead); err != nil {
		return nil, err
	}
	var list []*APIKey
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id, prefix, name, permissions, expires_at, last_used_at, created_at
			from api_keys where revoked_at is null order by created_at desc`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			k := &APIKey{}
			if err := rows.Scan(&k.ID, &k.Prefix, &k.Name, &k.Permissions, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt); err != nil {
				return err
			}
			list = append(list, k)
		}
		return rows.Err()
	})
	return list, err
}

// RevokeKey disables a key immediately. A scoped key may only revoke keys it
// could have created (no wider than itself).
func (s *Service) RevokeKey(ctx context.Context, p *Principal, id string) error {
	if err := p.Require(PermAPIKeyWrite); err != nil {
		return err
	}
	return s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var perms []string
		err := tx.QueryRow(ctx, "select permissions from api_keys where id::text = $1 and revoked_at is null", id).Scan(&perms)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("api key")
		}
		if err != nil {
			return err
		}
		if p.Permissions != nil {
			if perms == nil {
				return errf(http.StatusForbidden, "permission_escalation", "a scoped key cannot revoke a full access key")
			}
			for _, perm := range perms {
				if !p.Can(perm) {
					return errf(http.StatusForbidden, "permission_escalation", "cannot revoke a key holding %s", perm)
				}
			}
		}
		_, err = tx.Exec(ctx, "update api_keys set revoked_at = now() where id::text = $1", id)
		return err
	})
}

// AuditEntry is one security relevant action.
type AuditEntry struct {
	ID        int64           `json:"id"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	IP        string          `json:"ip"`
	RequestID string          `json:"request_id"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
}

// Audit records an action. Failures are returned so callers can log them;
// they never undo the action itself.
func (s *Service) Audit(ctx context.Context, orgID string, e AuditEntry) error {
	meta := e.Metadata
	if meta == nil {
		meta = json.RawMessage("{}")
	}
	return s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into audit_log (org_id, actor, action, target, ip, request_id, metadata)
			values ($1, $2, $3, $4, $5, $6, $7)`, orgID, e.Actor, e.Action, e.Target, e.IP, e.RequestID, meta)
		return err
	})
}

func (s *Service) ListAudit(ctx context.Context, p *Principal, limit int) ([]*AuditEntry, error) {
	if err := p.Require(PermAuditRead); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []*AuditEntry
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id, actor, action, target, ip, request_id, metadata, created_at
			from audit_log order by id desc limit $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e := &AuditEntry{}
			if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.IP, &e.RequestID, &e.Metadata, &e.CreatedAt); err != nil {
				return err
			}
			list = append(list, e)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Service) parentIsFull(ctx context.Context, orgID, parent string) bool {
	var full bool
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select permissions is null from api_keys where id = $1", parent).Scan(&full)
	})
	return err == nil && full
}

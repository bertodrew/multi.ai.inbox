package core

import (
	"context"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/keys"
)

// EventTypes lists every event a webhook can subscribe to.
var EventTypes = map[string]bool{
	"message.received":           true,
	"message.received.spam":      true,
	"message.sent":               true,
	"message.failed":             true,
	"message.security.completed": true,
	"message.security.review":    true,
}

type Webhook struct {
	ID         string    `json:"webhook_id"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	InboxIDs   []string  `json:"inbox_ids"`
	Enabled    bool      `json:"enabled"`
	Secret     string    `json:"secret,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateWebhookRequest struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	InboxIDs   []string `json:"inbox_ids"` // addresses
}

func (s *Service) CreateWebhook(ctx context.Context, orgID string, req CreateWebhookRequest) (*Webhook, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(s.AllowHTTPWebhooks && u.Scheme == "http")) {
		return nil, Validation("url must be an absolute https URL")
	}
	if len(req.EventTypes) == 0 {
		return nil, Validation("event_types must not be empty")
	}
	for _, t := range req.EventTypes {
		if !EventTypes[t] {
			return nil, Validation("unknown event type %q", t)
		}
	}
	if len(req.InboxIDs) > 10 {
		return nil, Validation("at most 10 inbox_ids")
	}
	secret, err := keys.NewWebhookSecret()
	if err != nil {
		return nil, err
	}
	w := &Webhook{URL: req.URL, EventTypes: req.EventTypes, InboxIDs: []string{}, Enabled: true, Secret: secret}
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		ids := []string{}
		for _, a := range req.InboxIDs {
			inbox, err := s.inboxTx(ctx, tx, a)
			if err != nil {
				return err
			}
			ids = append(ids, inbox.ID)
			w.InboxIDs = append(w.InboxIDs, inbox.InboxID)
		}
		return tx.QueryRow(ctx, `insert into webhooks (org_id, url, secret, event_types, inbox_ids)
			values ($1, $2, $3, $4, $5) returning id, created_at`, orgID, req.URL, secret, req.EventTypes, ids).Scan(&w.ID, &w.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (s *Service) ListWebhooks(ctx context.Context, orgID string) ([]*Webhook, error) {
	var list []*Webhook
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select w.id, w.url, w.event_types, w.enabled, w.created_at,
			coalesce((select array_agg(i.address::text) from inboxes i where i.id = any(w.inbox_ids)), '{}')
			from webhooks w order by w.created_at desc`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			w := &Webhook{}
			if err := rows.Scan(&w.ID, &w.URL, &w.EventTypes, &w.Enabled, &w.CreatedAt, &w.InboxIDs); err != nil {
				return err
			}
			list = append(list, w)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Service) DeleteWebhook(ctx context.Context, orgID, id string) error {
	return s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "delete from webhooks where id::text = $1", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return NotFound("webhook")
		}
		return nil
	})
}

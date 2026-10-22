package core

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Inbox struct {
	ID          string    `json:"id"`
	InboxID     string    `json:"inbox_id"` // the address, as in the reference API
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	ClientID    *string   `json:"client_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateInboxRequest struct {
	Username    string `json:"username"`
	Domain      string `json:"domain"`
	DisplayName string `json:"display_name"`
	ClientID    string `json:"client_id"`
}

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?$`)
	clientIDRe = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,256}$`)
)

const inboxCols = "id, address, display_name, client_id, created_at, updated_at"

func scanInbox(row pgx.Row) (*Inbox, error) {
	i := &Inbox{}
	if err := row.Scan(&i.ID, &i.InboxID, &i.DisplayName, &i.ClientID, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, err
	}
	i.Email = i.InboxID
	return i, nil
}

// CreateInbox returns (inbox, created). A repeated client_id returns the existing inbox.
func (s *Service) CreateInbox(ctx context.Context, orgID string, req CreateInboxRequest) (*Inbox, bool, error) {
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Domain == "" {
		req.Domain = s.SharedDomain
	}
	if req.Username == "" {
		req.Username = randomUsername()
	}
	if !usernameRe.MatchString(req.Username) {
		return nil, false, Validation("username must be 1 to 64 characters of a-z, 0-9, dot, dash, underscore")
	}
	if req.ClientID != "" && !clientIDRe.MatchString(req.ClientID) {
		return nil, false, Validation("client_id must be 1 to 256 characters of A-Z a-z 0-9 . _ ~ -")
	}
	if len(req.DisplayName) > 200 {
		return nil, false, Validation("display_name is longer than 200 characters")
	}
	var clientID any
	if req.ClientID != "" {
		clientID = req.ClientID
	}
	address := req.Username + "@" + req.Domain

	var out *Inbox
	created := false
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		if req.ClientID != "" {
			existing, err := scanInbox(tx.QueryRow(ctx, "select "+inboxCols+" from inboxes where client_id = $1", req.ClientID))
			if err == nil {
				out = existing
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		var domainID string
		err := tx.QueryRow(ctx, "select id from domains where name = $1", req.Domain).Scan(&domainID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusForbidden, "domain_not_verified", "domain %s is not available to this organization", req.Domain)
		}
		if err != nil {
			return err
		}
		out, err = scanInbox(tx.QueryRow(ctx, `insert into inboxes (org_id, domain_id, address, display_name, client_id)
			values ($1, $2, $3, $4, $5) returning `+inboxCols, orgID, domainID, address, req.DisplayName, clientID))
		if isUniqueViolation(err) {
			return errf(http.StatusConflict, "already_exists", "address %s is taken", address)
		}
		created = err == nil
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return out, created, nil
}

func (s *Service) ListInboxes(ctx context.Context, orgID string, p Page) ([]*Inbox, string, error) {
	cur, err := p.cursor()
	if err != nil {
		return nil, "", err
	}
	var list []*Inbox
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		q := "select " + inboxCols + " from inboxes"
		args := []any{p.limit() + 1}
		if cur != nil {
			q += " where (created_at, id) < ($2, $3)"
			args = append(args, cur.At, cur.ID)
		}
		rows, err := tx.Query(ctx, q+" order by created_at desc, id desc limit $1", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			i, err := scanInbox(rows)
			if err != nil {
				return err
			}
			list = append(list, i)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(list) > p.limit() {
		list = list[:p.limit()]
		last := list[len(list)-1]
		next = nextToken(last.CreatedAt, last.ID)
	}
	return list, next, nil
}

func (s *Service) GetInbox(ctx context.Context, orgID, address string) (*Inbox, error) {
	var out *Inbox
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		out, err = s.inboxTx(ctx, tx, address)
		return err
	})
	return out, err
}

func (s *Service) inboxTx(ctx context.Context, tx pgx.Tx, address string) (*Inbox, error) {
	i, err := scanInbox(tx.QueryRow(ctx, "select "+inboxCols+" from inboxes where address = $1", strings.ToLower(address)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NotFound("inbox")
	}
	return i, err
}

func (s *Service) UpdateInbox(ctx context.Context, orgID, address string, displayName *string) (*Inbox, error) {
	if displayName != nil && len(*displayName) > 200 {
		return nil, Validation("display_name is longer than 200 characters")
	}
	var out *Inbox
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		out, err = scanInbox(tx.QueryRow(ctx, `update inboxes set display_name = coalesce($2, display_name), updated_at = now()
			where address = $1 returning `+inboxCols, strings.ToLower(address), displayName))
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("inbox")
		}
		return err
	})
	return out, err
}

func (s *Service) DeleteInbox(ctx context.Context, orgID, address string) error {
	return s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "delete from inboxes where address = $1", strings.ToLower(address))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return NotFound("inbox")
		}
		return nil
	})
}

var (
	adjectives = []string{"amber", "brisk", "calm", "deft", "eager", "fleet", "gentle", "keen", "lucid", "nimble", "quiet", "swift", "tidy", "vivid", "wise"}
	nouns      = []string{"otter", "falcon", "maple", "harbor", "comet", "lynx", "pine", "orbit", "heron", "quartz", "willow", "badger", "ember", "fjord", "raven"}
)

func randomUsername() string {
	pick := func(list []string) string {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(list))))
		return list[n.Int64()]
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%s-%s-%04d", pick(adjectives), pick(nouns), n.Int64())
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

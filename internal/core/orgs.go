package core

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/keys"
)

type Org struct {
	ID   string `json:"organization_id"`
	Name string `json:"name"`
	Plan string `json:"plan"`
}

func (s *Service) CreateOrg(ctx context.Context, name string) (*Org, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, Validation("name must be 1 to 200 characters")
	}
	o := &Org{Name: name}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "insert into organizations (name) values ($1) returning id, plan", name).Scan(&o.ID, &o.Plan)
	})
	return o, err
}

type CreatedKey struct {
	ID     string `json:"api_key_id"`
	Key    string `json:"api_key"`
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

func (s *Service) CreateKey(ctx context.Context, orgID, name string) (*CreatedKey, error) {
	plain, prefix, hash, err := keys.New()
	if err != nil {
		return nil, err
	}
	k := &CreatedKey{Key: plain, Prefix: prefix, Name: name}
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "insert into api_keys (org_id, name, prefix, hash) values ($1, $2, $3, $4) returning id",
			orgID, name, prefix, hash).Scan(&k.ID)
	})
	return k, err
}

// Authenticate resolves an API key to its organization.
func (s *Service) Authenticate(ctx context.Context, plain string) (*Org, error) {
	if !keys.WellFormed(plain) {
		return nil, errf(http.StatusUnauthorized, "unknown_api_key", "invalid API key")
	}
	o := &Org{}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `update api_keys k set last_used_at = now()
			from organizations o
			where k.hash = $1 and k.revoked_at is null and o.id = k.org_id
			returning o.id, o.name, o.plan`, keys.Hash(plain)).Scan(&o.ID, &o.Name, &o.Plan)
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusUnauthorized, "unknown_api_key", "invalid API key")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Service) GetOrg(ctx context.Context, orgID string) (*Org, error) {
	o := &Org{}
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select id, name, plan from organizations where id = $1", orgID).Scan(&o.ID, &o.Name, &o.Plan)
	})
	return o, err
}

func (s *Service) plan(ctx context.Context, orgID string) (Plan, error) {
	o, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return Plan{}, err
	}
	return planOf(o.Plan), nil
}

// SetPlan changes an organization's plan (operator command, billing later).
func (s *Service) SetPlan(ctx context.Context, orgID, plan string) error {
	if _, ok := Plans[plan]; !ok {
		return Validation("unknown plan %q", plan)
	}
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "update organizations set plan = $2 where id = $1", orgID, plan)
		if err == nil && tag.RowsAffected() == 0 {
			return NotFound("organization")
		}
		return err
	})
}

// EnsureOrg returns the organization with this name, creating it if needed.
func (s *Service) EnsureOrg(ctx context.Context, name string) (*Org, bool, error) {
	o := &Org{}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select id, name, plan from organizations where name = $1 order by created_at limit 1", name).Scan(&o.ID, &o.Name, &o.Plan)
	})
	if err == nil {
		return o, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	o, err = s.CreateOrg(ctx, name)
	return o, err == nil, err
}

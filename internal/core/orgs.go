package core

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
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

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
	// ScreenBeforeNotify holds inbound mail until the gatekeeper verdict
	// exists: message.received fires only for clean mail or after a release.
	ScreenBeforeNotify bool `json:"screen_before_notify"`
}

const orgCols = "id, name, plan, screen_before_notify"

func scanOrg(row pgx.Row, o *Org) error {
	return row.Scan(&o.ID, &o.Name, &o.Plan, &o.ScreenBeforeNotify)
}

func (s *Service) CreateOrg(ctx context.Context, name string) (*Org, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, Validation("name must be 1 to 200 characters")
	}
	o := &Org{Name: name}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return scanOrg(tx.QueryRow(ctx, "insert into organizations (name) values ($1) returning "+orgCols, name), o)
	})
	return o, err
}

func (s *Service) GetOrg(ctx context.Context, orgID string) (*Org, error) {
	o := &Org{}
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return scanOrg(tx.QueryRow(ctx, "select "+orgCols+" from organizations where id = $1", orgID), o)
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

// SetScreening turns screen before notify on or off for an organization
// (operator command). Only system context may write organizations (RLS).
// Messages already held stay held until screened or released.
func (s *Service) SetScreening(ctx context.Context, orgID string, on bool) (*Org, error) {
	o := &Org{}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		err := scanOrg(tx.QueryRow(ctx, "update organizations set screen_before_notify = $2 where id::text = $1 returning "+orgCols, orgID, on), o)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("organization")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// EnsureOrg returns the organization with this name, creating it if needed.
func (s *Service) EnsureOrg(ctx context.Context, name string) (*Org, bool, error) {
	o := &Org{}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return scanOrg(tx.QueryRow(ctx, "select "+orgCols+" from organizations where name = $1 order by created_at limit 1", name), o)
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

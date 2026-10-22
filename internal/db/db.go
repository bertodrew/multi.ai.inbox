// Package db wraps the Postgres pool and scopes every transaction to a tenant
// or to the system context, so row level security always applies.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) Close() { d.Pool.Close() }

// WithOrg runs fn in a transaction where only rows of orgID are visible.
func (d *DB) WithOrg(ctx context.Context, orgID string, fn func(pgx.Tx) error) error {
	return d.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.org_id', $1, true)", orgID); err != nil {
			return fmt.Errorf("set tenant: %w", err)
		}
		return fn(tx)
	})
}

// WithSystem runs fn with cross tenant access. Only the SMTP edge, the
// workers and authentication use it.
func (d *DB) WithSystem(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.system', 'on', true)"); err != nil {
			return fmt.Errorf("set system: %w", err)
		}
		return fn(tx)
	})
}

func (d *DB) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Migrate applies embedded migrations in lexical order, once each.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.Pool.Exec(ctx, `create table if not exists schema_migrations (
		name text primary key, applied_at timestamptz not null default now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		short := strings.TrimPrefix(name, "migrations/")
		var exists bool
		if err := d.Pool.QueryRow(ctx, "select exists(select 1 from schema_migrations where name = $1)", short).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		err = d.tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return fmt.Errorf("apply %s: %w", short, err)
			}
			_, err := tx.Exec(ctx, "insert into schema_migrations (name) values ($1)", short)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

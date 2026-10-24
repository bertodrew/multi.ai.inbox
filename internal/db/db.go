// Package db wraps the Postgres pools and scopes every transaction to a
// tenant or to the system context, so row level security always applies.
//
// Production uses three roles (deploy/k8s/base/postgres-roles.sql): mai_owner
// runs migrations, mai_app serves tenant transactions and mai_system serves
// system transactions. Row level security honours app.system only for members
// of mai_system, so SQL running as mai_app cannot reach other tenants.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	// Pool serves tenant transactions (role mai_app in production).
	Pool *pgxpool.Pool
	// System serves system transactions (role mai_system in production).
	// Nil means Pool is used for both, which only works when that role may
	// use the system context (a superuser or mai_system in development).
	System *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*DB, error) {
	pool, err := connect(ctx, url)
	if err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

// OpenSystem connects the pool used by WithSystem.
func (d *DB) OpenSystem(ctx context.Context, url string) error {
	pool, err := connect(ctx, url)
	if err != nil {
		return fmt.Errorf("system: %w", err)
	}
	if d.System != nil {
		d.System.Close()
	}
	d.System = pool
	return nil
}

func connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// Every timestamp we return is UTC, whatever time zone the host happens to
	// be in. Without this the same instant is serialized as +01:00 on a machine
	// in Zurich and as Z in a container, which is a difference no client should
	// ever have to see.
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	// The session parameter alone is not enough: pgx decodes timestamptz into
	// time.Local unless it is told otherwise, so the same instant would be
	// serialized as +02:00 on a laptop and as Z in a container. Scan in UTC.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name:  "timestamptz",
			OID:   pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func (d *DB) Close() {
	d.Pool.Close()
	if d.System != nil {
		d.System.Close()
	}
}

func (d *DB) systemPool() *pgxpool.Pool {
	if d.System != nil {
		return d.System
	}
	return d.Pool
}

// CheckSystem fails when WithSystem would not actually get cross tenant
// access, for example when the system pool connects as mai_app. Call it after
// Migrate.
func (d *DB) CheckSystem(ctx context.Context) error {
	var user string
	var ok bool
	err := d.systemPool().QueryRow(ctx, `select current_user::text, case
		when exists (select from pg_roles where rolname = 'mai_system') then pg_has_role(current_user, 'mai_system', 'member')
		else (select rolsuper from pg_roles where rolname = current_user) end`).Scan(&user, &ok)
	if err != nil {
		return fmt.Errorf("check system role: %w", err)
	}
	if !ok {
		return fmt.Errorf("role %s cannot use the system context: set MAI_SYSTEM_DATABASE_URL to a mai_system login", user)
	}
	return nil
}

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
// workers and authentication use it. It runs on the system pool: the setting
// alone grants nothing to roles outside mai_system.
func (d *DB) WithSystem(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.txOn(ctx, d.systemPool(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.system', 'on', true)"); err != nil {
			return fmt.Errorf("set system: %w", err)
		}
		return fn(tx)
	})
}

func (d *DB) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.txOn(ctx, d.Pool, fn)
}

func (d *DB) txOn(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Migrate applies embedded migrations in lexical order, once each. It must
// run as the schema owner (mai_owner, see MAI_MIGRATE_DATABASE_URL). When
// nothing is pending it changes nothing, so the runtime roles can call it to
// check that the schema is current.
func (d *DB) Migrate(ctx context.Context) error {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	var tracked bool
	if err := d.Pool.QueryRow(ctx, "select to_regclass('schema_migrations') is not null").Scan(&tracked); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	if tracked {
		var applied int
		if err := d.Pool.QueryRow(ctx, "select count(*) from schema_migrations where name = any($1)", shortNames(names)).Scan(&applied); err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}
		if applied == len(names) {
			return nil
		}
	}
	if _, err := d.Pool.Exec(ctx, `create table if not exists schema_migrations (
		name text primary key, applied_at timestamptz not null default now())`); err != nil {
		return fmt.Errorf("create schema_migrations (migrations must run as the schema owner, see MAI_MIGRATE_DATABASE_URL): %w", err)
	}
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
			// Data migrations see every tenant (the owner is a member of mai_system).
			if _, err := tx.Exec(ctx, "select set_config('app.system', 'on', true)"); err != nil {
				return err
			}
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

func shortNames(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = strings.TrimPrefix(n, "migrations/")
	}
	return out
}

package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ynov/equipment-loans/migrations"
)

const migrationLockKey = 727274001

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("postgres: acquire connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("postgres: migration lock: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	if _, err := conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())"); err != nil {
		return fmt.Errorf("postgres: create schema_migrations: %w", err)
	}
	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := applyMigration(ctx, conn, name); err != nil {
			return err
		}
	}
	return nil
}

func migrationNames() ([]string, error) {
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("postgres: list migrations: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, name string) error {
	var applied bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", name).Scan(&applied); err != nil {
		return fmt.Errorf("postgres: check migration %s: %w", name, err)
	}
	if applied {
		return nil
	}
	script, err := migrations.FS.ReadFile(name)
	if err != nil {
		return fmt.Errorf("postgres: read migration %s: %w", name, err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin migration %s: %w", name, err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if err := runMigration(ctx, tx, name, string(script)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit migration %s: %w", name, err)
	}
	return nil
}

func runMigration(ctx context.Context, tx pgx.Tx, name, script string) error {
	if _, err := tx.Exec(ctx, script); err != nil {
		return fmt.Errorf("postgres: apply migration %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", name); err != nil {
		return fmt.Errorf("postgres: record migration %s: %w", name, err)
	}
	return nil
}

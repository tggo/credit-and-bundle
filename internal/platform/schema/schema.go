// Package schema owns the SQL schema and seed of the simulator.
package schema

import (
	"context"
	_ "embed"

	"bundle-credit-test/internal/platform/db"
)

//go:embed schema.sql
var sql string

//go:embed comments.sql
var comments string

// Describe applies the table and column descriptions.
func Describe(ctx context.Context, q db.Q) error {
	_, err := q.Exec(ctx, comments)
	return err
}

// Reset drops and recreates the simulator tables with the seed.
func Reset(ctx context.Context, q db.Q) error {
	_, err := q.Exec(ctx, sql)
	return err
}

// Seeded tells whether the current schema exists already (credit_lots.revoked_at is the marker of this version).
func Seeded(ctx context.Context, q db.Q) bool {
	var ok bool
	_ = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='credit_lots' AND column_name='revoked_at')`).Scan(&ok)
	return ok
}

// EnsureBoards creates the board storage, which survives resets.
func EnsureBoards(ctx context.Context, q db.Q) error {
	_, err := q.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS boards;
		CREATE TABLE IF NOT EXISTS boards.scenes (id text PRIMARY KEY, scene jsonb NOT NULL, updated_at timestamptz NOT NULL);
		ALTER TABLE boards.scenes ADD COLUMN IF NOT EXISTS rev bigint NOT NULL DEFAULT 0;
		CREATE TABLE IF NOT EXISTS boards.diagrams (id text PRIMARY KEY, source text NOT NULL, rev bigint NOT NULL, updated_at timestamptz NOT NULL)`)
	return err
}

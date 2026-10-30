// Package storages keeps board scenes in Postgres (schema boards, survives simulator resets).
package storages

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"bundle-credit-test/internal/platform/db"
)

type Store struct{ db db.Q }

func New(q db.Q) *Store { return &Store{db: q} }

func (s *Store) Get(ctx context.Context, id string) ([]byte, int64, error) {
	var scene []byte
	var rev int64
	err := s.db.QueryRow(ctx, `SELECT scene, rev FROM boards.scenes WHERE id=$1`, id).Scan(&scene, &rev)
	return scene, rev, err
}

// Save stores the scene and bumps the revision. An identical scene (compared as jsonb) is not written:
// changed is false and rev stays, so the caller does not broadcast it.
func (s *Store) Save(ctx context.Context, id string, scene []byte) (rev int64, changed bool, err error) {
	err = s.db.QueryRow(ctx, `INSERT INTO boards.scenes (id, scene, updated_at, rev) VALUES ($1, $2::jsonb, now(), 1)
		ON CONFLICT (id) DO UPDATE SET scene = EXCLUDED.scene, updated_at = now(), rev = boards.scenes.rev + 1
		WHERE boards.scenes.scene IS DISTINCT FROM EXCLUDED.scene
		RETURNING rev`, id, string(scene)).Scan(&rev)
	if errors.Is(err, pgx.ErrNoRows) {
		_, rev, err = s.Get(ctx, id)
		return rev, false, err
	}
	return rev, err == nil, err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM boards.scenes WHERE id=$1`, id)
	return err
}

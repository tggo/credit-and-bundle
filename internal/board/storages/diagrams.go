package storages

import "context"

func (s *Store) DiagramSource(ctx context.Context, id string) (string, int64, error) {
	var src string
	var rev int64
	err := s.db.QueryRow(ctx, `SELECT source, rev FROM boards.diagrams WHERE id=$1`, id).Scan(&src, &rev)
	return src, rev, err
}

func (s *Store) SaveDiagram(ctx context.Context, id, source string) (int64, error) {
	var rev int64
	err := s.db.QueryRow(ctx, `INSERT INTO boards.diagrams (id, source, rev, updated_at) VALUES ($1, $2, 1, now())
		ON CONFLICT (id) DO UPDATE SET source = EXCLUDED.source, rev = boards.diagrams.rev + 1, updated_at = now()
		RETURNING rev`, id, source).Scan(&rev)
	return rev, err
}

func (s *Store) DeleteDiagram(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM boards.diagrams WHERE id=$1`, id)
	return err
}

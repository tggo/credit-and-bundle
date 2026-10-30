// Package storages reads customers from Postgres.
package storages

import (
	"context"

	"bundle-credit-test/internal/customer/models"
	"bundle-credit-test/internal/platform/db"
)

type Store struct{ db db.Q }

func New(q db.Q) *Store { return &Store{db: q} }

const cols = `id::text, handle, email, name`

func (s *Store) ByHandle(ctx context.Context, handle string) (models.Customer, error) {
	var c models.Customer
	err := s.db.QueryRow(ctx, `SELECT `+cols+` FROM customers WHERE handle=$1`, handle).Scan(&c.ID, &c.Handle, &c.Email, &c.Name)
	return c, err
}

func (s *Store) ByID(ctx context.Context, id string) (models.Customer, error) {
	var c models.Customer
	err := s.db.QueryRow(ctx, `SELECT `+cols+` FROM customers WHERE id::text=$1`, id).Scan(&c.ID, &c.Handle, &c.Email, &c.Name)
	return c, err
}

func (s *Store) List(ctx context.Context) ([]models.Customer, error) {
	rows, err := s.db.Query(ctx, `SELECT `+cols+` FROM customers ORDER BY handle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Customer
	for rows.Next() {
		var c models.Customer
		if err := rows.Scan(&c.ID, &c.Handle, &c.Email, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

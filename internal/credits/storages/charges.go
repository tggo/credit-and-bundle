package storages

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

// InsertCharge records the charge as in_progress. inserted=false: the idempotency key exists already.
func (s *Store) InsertCharge(ctx context.Context, q db.Q, accountID string, r models.ChargeRequest, price int64, reqHash string, at time.Time) (string, bool, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO credit_charges (account_id, application_id, action_code, amount, status, idempotency_key, request_hash, created_at)
		VALUES ($1,$2,$3,$4,'in_progress',$5,$6,$7) ON CONFLICT (account_id, idempotency_key) DO NOTHING RETURNING id::text`,
		accountID, r.AppID, r.Action, price, r.Key, reqHash, at).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (s *Store) StoredCharge(ctx context.Context, accountID, key string) (id, status, hash string, amount int64, result []byte, err error) {
	err = s.db.QueryRow(ctx, `SELECT id::text, status, request_hash, amount, result FROM credit_charges WHERE account_id=$1 AND idempotency_key=$2`,
		accountID, key).Scan(&id, &status, &hash, &amount, &result)
	return
}

// CompleteCharge finishes an in_progress charge; false means the sweeper already refunded it.
func (s *Store) CompleteCharge(ctx context.Context, chargeID string, result []byte, cost int64) (bool, error) {
	t, err := s.db.Exec(ctx, `UPDATE credit_charges SET status='completed', result=$2::jsonb, cost_minor=$3
		WHERE id=$1 AND status='in_progress'`, chargeID, string(result), cost)
	return t.RowsAffected() == 1, err
}

// ClaimRefund moves an in_progress charge to refunded; false means someone else already finished it.
func (s *Store) ClaimRefund(ctx context.Context, q db.Q, chargeID string) (bool, error) {
	t, err := q.Exec(ctx, `UPDATE credit_charges SET status='refunded' WHERE id=$1 AND status='in_progress'`, chargeID)
	return t.RowsAffected() == 1, err
}

// StaleCharges are charges still in_progress that started before the cutoff (the request died).
func (s *Store) StaleCharges(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text FROM credit_charges WHERE status='in_progress' AND created_at < $1 ORDER BY created_at`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

package storages

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

func scanLots(rows pgx.Rows) ([]models.LotLeft, error) {
	defer rows.Close()
	var out []models.LotLeft
	for rows.Next() {
		var l models.LotLeft
		if err := rows.Scan(&l.ID, &l.AccountID, &l.Label, &l.Remaining); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) EligibleLots(ctx context.Context, q db.Q, accountID, appID string, at time.Time) ([]models.LotLeft, error) {
	rows, err := q.Query(ctx, eligibleLotsSQL, accountID, appID, at)
	if err != nil {
		return nil, err
	}
	return scanLots(rows)
}

// Debit takes credits from a lot only if it still has them.
func (s *Store) Debit(ctx context.Context, q db.Q, lotID string, amount int64) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE credit_lots SET remaining = remaining - $2 WHERE id=$1 AND remaining >= $2`, lotID, amount)
	return err == nil && tag.RowsAffected() == 1, err
}

func (s *Store) Credit(ctx context.Context, q db.Q, lotID string, amount int64) error {
	_, err := q.Exec(ctx, `UPDATE credit_lots SET remaining = remaining + $2 WHERE id=$1`, lotID, amount)
	return err
}

// InsertLot creates the lot of a payment item and its app scope; inserted=false when this payment already granted it.
func (s *Store) InsertLot(ctx context.Context, q db.Q, accountID string, g models.Grant) (string, bool, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO credit_lots (account_id, source_type, payment_id, offering_item_id, subscription_id, label, scope, kind, granted, remaining, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'paid',$8,$8,$9,$10)
		ON CONFLICT (payment_id, offering_item_id) DO NOTHING RETURNING id::text`,
		accountID, g.Source, g.PaymentID, g.OfferingItemID, g.SubscriptionID, g.Label, g.Scope, g.Credits, g.Expires, g.At).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, app := range g.Apps {
		if _, err := q.Exec(ctx, `INSERT INTO lot_scope_apps VALUES ($1,$2)`, id, app); err != nil {
			return "", false, err
		}
	}
	return id, true, nil
}

// ZeroSubscriptionLots empties every lot of the subscription and returns what each had.

// ZeroPaymentLots empties and marks revoked every lot a payment granted, also those already at 0 (a running charge
// may return credits to them later), and returns what each had (refund of that payment).
func (s *Store) ZeroPaymentLots(ctx context.Context, q db.Q, paymentID string, at time.Time) ([]models.LotLeft, error) {
	rows, err := q.Query(ctx, `UPDATE credit_lots l SET remaining = 0, revoked_at = $2
		FROM (SELECT id, remaining FROM credit_lots WHERE payment_id=$1 AND revoked_at IS NULL FOR UPDATE) old
		WHERE l.id = old.id RETURNING l.id::text, l.account_id::text, l.label, old.remaining`, paymentID, at)
	if err != nil {
		return nil, err
	}
	return scanLots(rows)
}

// ZeroExpiredLots empties every lot past its expiry and returns what each had.
func (s *Store) ZeroExpiredLots(ctx context.Context, q db.Q, at time.Time) ([]models.LotLeft, error) {
	rows, err := q.Query(ctx, `UPDATE credit_lots l SET remaining = 0
		FROM (SELECT id, remaining FROM credit_lots WHERE remaining > 0 AND expires_at <= $1 FOR UPDATE) old
		WHERE l.id = old.id RETURNING l.id::text, l.account_id::text, l.label, old.remaining`, at)
	if err != nil {
		return nil, err
	}
	return scanLots(rows)
}

func (s *Store) PaymentUsage(ctx context.Context, paymentID string) (models.Usage, error) {
	var u models.Usage
	err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(granted),0), COALESCE(SUM(remaining),0) FROM credit_lots WHERE payment_id=$1`, paymentID).
		Scan(&u.Granted, &u.Remaining)
	return u, err
}

// LotState tells whether a lot's payment was refunded and when it expires.
func (s *Store) LotState(ctx context.Context, q db.Q, lotID string) (revoked bool, expires time.Time, err error) {
	err = q.QueryRow(ctx, `SELECT revoked_at IS NOT NULL, expires_at FROM credit_lots WHERE id=$1`, lotID).
		Scan(&revoked, &expires)
	return
}

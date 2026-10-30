package storages

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"bundle-credit-test/internal/billing/models"
	"bundle-credit-test/internal/platform/db"
)

const subCols = `id::text, provider_sub_ref, customer_id::text, offering_id::text, kind, status, current_period_end`

func scanSub(row pgx.Row) (models.Subscription, error) {
	var s models.Subscription
	err := row.Scan(&s.ID, &s.Ref, &s.CustomerID, &s.OfferingID, &s.Kind, &s.Status, &s.PeriodEnd)
	return s, err
}

// LockSubscription returns nil when the provider reference is unknown.
func (s *Store) LockSubscription(ctx context.Context, q db.Q, subRef string) (*models.Subscription, error) {
	sub, err := scanSub(q.QueryRow(ctx, `SELECT `+subCols+` FROM subscriptions WHERE provider_sub_ref=$1 FOR UPDATE`, subRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &sub, err
}

func (s *Store) InsertSubscription(ctx context.Context, q db.Q, ev models.Event, kind string, at time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO subscriptions (provider_sub_ref, customer_id, offering_id, kind, status, created_at)
		VALUES ($1,$2,$3,$4,'created',$5)`, ev.SubRef, ev.CustomerID, ev.OfferingID, kind, at)
	return err
}

func (s *Store) SetSubscriptionStatus(ctx context.Context, q db.Q, id, status string) error {
	_, err := q.Exec(ctx, `UPDATE subscriptions SET status=$2 WHERE id=$1`, id, status)
	return err
}

func (s *Store) ActivatePeriod(ctx context.Context, q db.Q, id string, start, end time.Time) error {
	_, err := q.Exec(ctx, `UPDATE subscriptions SET status='active', current_period_start=$2, current_period_end=$3 WHERE id=$1`, id, start, end)
	return err
}

func (s *Store) Subscription(ctx context.Context, id string) (models.Subscription, error) {
	return scanSub(s.db.QueryRow(ctx, `SELECT `+subCols+` FROM subscriptions WHERE id::text=$1`, id))
}

func (s *Store) LatestSubscription(ctx context.Context, customerID string, activeRecurring bool) (models.Subscription, error) {
	where := ""
	if activeRecurring {
		where = ` AND status='active' AND kind='recurring'`
	}
	return scanSub(s.db.QueryRow(ctx, `SELECT `+subCols+` FROM subscriptions WHERE customer_id::text=$1`+where+` ORDER BY created_at DESC LIMIT 1`, customerID))
}

func (s *Store) DueRenewals(ctx context.Context, at time.Time) ([]models.Subscription, error) {
	rows, err := s.db.Query(ctx, `SELECT `+subCols+` FROM subscriptions WHERE kind='recurring' AND status='active' AND current_period_end <= $1`, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Subscription
	for rows.Next() {
		sub, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) SubViews(ctx context.Context, customerID string) ([]models.SubView, error) {
	where, args := "", []any{}
	if customerID != "" {
		where, args = ` WHERE s.customer_id::text=$1`, []any{customerID}
	}
	rows, err := s.db.Query(ctx, `SELECT s.id::text, s.provider_sub_ref, o.name, s.kind, s.status, s.current_period_end
		FROM subscriptions s JOIN offerings o ON o.id=s.offering_id`+where+` ORDER BY s.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SubView
	for rows.Next() {
		var v models.SubView
		if err := rows.Scan(&v.ID, &v.Ref, &v.Offering, &v.Kind, &v.Status, &v.End); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

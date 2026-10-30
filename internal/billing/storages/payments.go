package storages

import (
	"context"
	"time"

	"bundle-credit-test/internal/billing/models"
	"bundle-credit-test/internal/platform/db"
)

func (s *Store) PaymentExists(ctx context.Context, q db.Q, ref string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payments WHERE payment_ref=$1)`, ref).Scan(&ok)
	return ok, err
}

func (s *Store) InsertPayment(ctx context.Context, q db.Q, subID string, ev models.Event, at time.Time) (string, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO payments (subscription_id, payment_ref, amount_minor, currency, period_start, period_end, status, created_at)
		VALUES ($1,$2,$3,'USD',$4,$5,'succeeded',$6) RETURNING id::text`, subID, ev.PaymentRef, ev.AmountMinor, ev.PeriodStart, ev.PeriodEnd, at).Scan(&id)
	return id, err
}

func (s *Store) LockPayment(ctx context.Context, q db.Q, ref string) (id, subID, status string, err error) {
	err = q.QueryRow(ctx, `SELECT id::text, subscription_id::text, status FROM payments WHERE payment_ref=$1 FOR UPDATE`, ref).Scan(&id, &subID, &status)
	return
}

func (s *Store) RefundPayment(ctx context.Context, q db.Q, id string, amount int64) error {
	_, err := q.Exec(ctx, `UPDATE payments SET status='refunded', refunded_minor=$2 WHERE id=$1`, id, amount)
	return err
}

// PaidUntil is the end of the latest period still paid (succeeded payments only); nil when none is left.
func (s *Store) PaidUntil(ctx context.Context, q db.Q, subscriptionID string) (*time.Time, error) {
	var t *time.Time
	err := q.QueryRow(ctx, `SELECT max(period_end) FROM payments WHERE subscription_id=$1 AND status='succeeded'`, subscriptionID).Scan(&t)
	return t, err
}

func (s *Store) LatestPaid(ctx context.Context, subscriptionID string) (models.Paid, error) {
	var p models.Paid
	err := s.db.QueryRow(ctx, `SELECT p.id::text, p.payment_ref, s.offering_id::text, p.amount_minor, p.period_start, p.period_end
		FROM payments p JOIN subscriptions s ON s.id=p.subscription_id
		WHERE p.subscription_id::text=$1 AND p.status='succeeded' ORDER BY p.period_start DESC LIMIT 1`, subscriptionID).
		Scan(&p.ID, &p.Ref, &p.OfferingID, &p.Amount, &p.Start, &p.End)
	return p, err
}

// LatestCustomerPayment returns the customer's newest payment and its subscription's provider reference.
func (s *Store) LatestCustomerPayment(ctx context.Context, customerID string) (models.Paid, string, error) {
	var p models.Paid
	var subRef string
	err := s.db.QueryRow(ctx, `SELECT p.id::text, p.payment_ref, s.offering_id::text, p.amount_minor, p.period_start, p.period_end, s.provider_sub_ref
		FROM payments p JOIN subscriptions s ON s.id=p.subscription_id WHERE s.customer_id::text=$1 ORDER BY p.created_at DESC LIMIT 1`, customerID).
		Scan(&p.ID, &p.Ref, &p.OfferingID, &p.Amount, &p.Start, &p.End, &subRef)
	return p, subRef, err
}

// PeriodStartBefore is where a replayed payment's period starts: the end of the last period before it, or the given time.
func (s *Store) PeriodStartBefore(ctx context.Context, subRef string, before time.Time) (time.Time, error) {
	var start time.Time
	err := s.db.QueryRow(ctx, `SELECT COALESCE(MAX(period_end), $2) FROM payments p JOIN subscriptions s ON s.id=p.subscription_id
		WHERE s.provider_sub_ref=$1 AND p.period_end <= $2`, subRef, before).Scan(&start)
	return start, err
}

// InjectPhantom records a payment the provider does not know (simulated bug).
func (s *Store) InjectPhantom(ctx context.Context, ref string, at time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO payments (subscription_id, payment_ref, amount_minor, currency, period_start, period_end, status, created_at)
		SELECT subscription_id, $1, 999, 'USD', period_start, period_end, 'succeeded', $2 FROM payments ORDER BY created_at DESC LIMIT 1`, ref, at)
	return err
}

func (s *Store) PaymentStatus(ctx context.Context, ref string) (*string, error) {
	var st *string
	err := s.db.QueryRow(ctx, `SELECT status FROM payments WHERE payment_ref=$1`, ref).Scan(&st)
	if err != nil {
		return nil, nil
	}
	return st, nil
}

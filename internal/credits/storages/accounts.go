package storages

import (
	"context"
	"time"

	"bundle-credit-test/internal/platform/db"
)

func (s *Store) Account(ctx context.Context, q db.Q, customerID string) (id, status string, err error) {
	err = q.QueryRow(ctx, `SELECT id::text, status FROM credit_accounts WHERE customer_id::text=$1`, customerID).Scan(&id, &status)
	return
}

// LockAccount serializes a customer's charges. It must come before any insert that references the account:
// such an insert takes a key-share lock on the same row, and taking FOR UPDATE after it deadlocks under a race.
func (s *Store) LockAccount(ctx context.Context, q db.Q, accountID string) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM credit_accounts WHERE id=$1 FOR UPDATE`, accountID)
	return err
}

func (s *Store) Available(ctx context.Context, accountID, appID string, at time.Time) int64 {
	var n int64
	_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(remaining),0) FROM (`+eligibleLotsSQL+`) x`, accountID, appID, at).Scan(&n)
	return n
}

// LockChargeAccount locks the account of a charge, for refunds: same order as a charge (account first, then lots).
func (s *Store) LockChargeAccount(ctx context.Context, q db.Q, chargeID string) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM credit_accounts WHERE id=(SELECT account_id FROM credit_charges WHERE id=$1) FOR UPDATE`, chargeID)
	return err
}

// LockPaymentAccounts locks the accounts holding lots of a payment, in id order, before those lots are revoked.
func (s *Store) LockPaymentAccounts(ctx context.Context, q db.Q, paymentID string) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM credit_accounts WHERE id IN (SELECT account_id FROM credit_lots WHERE payment_id=$1) ORDER BY id FOR UPDATE`, paymentID)
	return err
}

// LockExpiredAccounts locks, in id order, every account with a lot to burn at this time.
func (s *Store) LockExpiredAccounts(ctx context.Context, q db.Q, at time.Time) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM credit_accounts WHERE id IN (SELECT account_id FROM credit_lots WHERE remaining > 0 AND expires_at <= $1) ORDER BY id FOR UPDATE`, at)
	return err
}

package storages

import (
	"context"
	"fmt"
	"time"

	"bundle-credit-test/internal/billing/models"
)

// The mock payment provider's own table: the other side of reconciliation.

func (s *Store) ProviderCharge(ctx context.Context, ref, subRef, customerID, offeringID string, amount int64, at time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO provider_txns (payment_ref, provider_sub_ref, customer_id, offering_id, amount_minor, currency, status, created_at)
		VALUES ($1,$2,$3,$4,$5,'USD','succeeded',$6)`, ref, subRef, customerID, offeringID, amount, at)
	return err
}

func (s *Store) ProviderRefund(ctx context.Context, ref string, amount int64) error {
	_, err := s.db.Exec(ctx, `UPDATE provider_txns SET status='refunded', refunded_minor=$2 WHERE payment_ref=$1`, ref, amount)
	return err
}

func (s *Store) ProviderTxn(ctx context.Context, ref string) (models.ProviderTxn, error) {
	var t models.ProviderTxn
	err := s.db.QueryRow(ctx, `SELECT payment_ref, provider_sub_ref, customer_id::text, offering_id::text, status, refunded_minor, created_at
		FROM provider_txns WHERE payment_ref=$1`, ref).Scan(&t.Ref, &t.SubRef, &t.CustomerID, &t.OfferingID, &t.Status, &t.Refunded, &t.Created)
	return t, err
}

// ProviderSilentRefund refunds the newest provider payment without sending a webhook (simulated).
func (s *Store) ProviderSilentRefund(ctx context.Context) (string, error) {
	var ref string
	err := s.db.QueryRow(ctx, `UPDATE provider_txns SET status='refunded', refunded_minor=amount_minor
		WHERE payment_ref = (SELECT payment_ref FROM provider_txns WHERE status='succeeded' ORDER BY created_at DESC LIMIT 1)
		RETURNING payment_ref`).Scan(&ref)
	return ref, err
}

// Reconcile matches every provider transaction with our payment by payment_ref.
func (s *Store) Reconcile(ctx context.Context) ([]models.ReconRow, int, error) {
	rows, err := s.db.Query(ctx, `SELECT COALESCE(t.payment_ref, p.payment_ref), t.status, p.status, t.amount_minor, p.amount_minor
		FROM provider_txns t FULL OUTER JOIN payments p ON p.payment_ref = t.payment_ref ORDER BY 1`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.ReconRow
	n := 0
	for rows.Next() {
		n++
		var ref string
		var ts, ps *string
		var ta, pa *int64
		if err := rows.Scan(&ref, &ts, &ps, &ta, &pa); err != nil {
			return nil, 0, err
		}
		switch {
		case ps == nil:
			out = append(out, models.ReconRow{Kind: "missing_ours", PaymentRef: ref, Fixable: true,
				Detail: "provider charged, we never recorded the payment (lost webhook) — customer paid but has no rights"})
		case ts == nil:
			out = append(out, models.ReconRow{Kind: "missing_provider", PaymentRef: ref,
				Detail: "we recorded a payment the provider does not know — rights granted without money"})
		case *ts != *ps:
			out = append(out, models.ReconRow{Kind: "status_mismatch", PaymentRef: ref, Fixable: *ts == "refunded",
				Detail: fmt.Sprintf("provider says %s, we say %s — refund webhook never came, customer got the money back and still has the rights", *ts, *ps)})
		case *ta != *pa:
			out = append(out, models.ReconRow{Kind: "amount_mismatch", PaymentRef: ref,
				Detail: fmt.Sprintf("provider: %d, ours: %d", *ta, *pa)})
		}
	}
	return out, n, rows.Err()
}

func (s *Store) NetTotals(ctx context.Context) (provider, ours int64) {
	_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount_minor - refunded_minor),0) FROM provider_txns`).Scan(&provider)
	_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount_minor - refunded_minor),0) FROM payments`).Scan(&ours)
	return
}

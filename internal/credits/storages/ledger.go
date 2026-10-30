package storages

import (
	"context"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

// Write appends one ledger row. The ledger is never updated or deleted.
func (s *Store) Write(ctx context.Context, q db.Q, e models.Entry) error {
	var charge *string
	if e.ChargeID != "" {
		charge = &e.ChargeID
	}
	_, err := q.Exec(ctx, `INSERT INTO credit_ledger (account_id, lot_id, charge_id, type, delta, note, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.AccountID, e.LotID, charge, e.Type, e.Delta, e.Note, e.At)
	return err
}

// ChargeEntries are the debits of one charge, as positive amounts per lot.
func (s *Store) ChargeEntries(ctx context.Context, q db.Q, chargeID string) ([]models.Entry, error) {
	rows, err := q.Query(ctx, `SELECT account_id::text, lot_id::text, -delta FROM credit_ledger WHERE charge_id=$1 AND type='charge'`, chargeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Entry
	for rows.Next() {
		e := models.Entry{ChargeID: chargeID}
		if err := rows.Scan(&e.AccountID, &e.LotID, &e.Delta); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

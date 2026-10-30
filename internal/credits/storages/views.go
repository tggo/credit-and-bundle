package storages

import (
	"context"
	"time"

	"bundle-credit-test/internal/credits/models"
)

func (s *Store) Balance(ctx context.Context, accountID, appID string, at time.Time) (models.Balance, error) {
	rows, err := s.db.Query(ctx, `SELECT l.label, l.source_type, l.scope, l.kind, l.remaining, l.expires_at,
		(l.scope='platform' OR EXISTS (SELECT 1 FROM lot_scope_apps a WHERE a.lot_id=l.id AND a.application_id::text=$2))
		FROM credit_lots l WHERE l.account_id=$1 AND l.remaining > 0 AND l.expires_at > $3 ORDER BY l.expires_at`, accountID, appID, at)
	if err != nil {
		return models.Balance{}, err
	}
	defer rows.Close()
	b := models.Balance{ApplicationID: appID, BySource: []models.BalanceSource{}}
	for rows.Next() {
		var x models.BalanceSource
		if err := rows.Scan(&x.Label, &x.Source, &x.Scope, &x.Kind, &x.Remaining, &x.ExpiresAt, &x.AppliesHere); err != nil {
			return b, err
		}
		b.Total += x.Remaining
		if x.AppliesHere || appID == "" {
			b.Available += x.Remaining
		}
		b.BySource = append(b.BySource, x)
	}
	return b, rows.Err()
}

func (s *Store) Lots(ctx context.Context, accountID string, at time.Time) ([]models.Lot, error) {
	rows, err := s.db.Query(ctx, `SELECT l.label, l.scope, COALESCE(string_agg(ap.name, ', ' ORDER BY ap.name), 'all apps'),
		l.kind, l.source_type, l.granted, l.remaining, l.expires_at,
		l.revoked_at IS NOT NULL
		FROM credit_lots l LEFT JOIN lot_scope_apps a ON a.lot_id=l.id LEFT JOIN applications ap ON ap.id = a.application_id
		WHERE l.account_id=$1 GROUP BY l.id ORDER BY l.created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Lot
	for rows.Next() {
		var v models.Lot
		if err := rows.Scan(&v.Label, &v.Scope, &v.Apps, &v.Kind, &v.Source, &v.Granted, &v.Remaining, &v.Expires, &v.Revoked); err != nil {
			return nil, err
		}
		v.Expired = !v.Expires.After(at)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Ledger(ctx context.Context, accountID string, limit int) ([]models.LedgerEntry, error) {
	rows, err := s.db.Query(ctx, `SELECT e.created_at, e.type, l.label, e.delta, e.note FROM credit_ledger e JOIN credit_lots l ON l.id=e.lot_id
		WHERE e.account_id=$1 ORDER BY e.id DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.LedgerEntry
	for rows.Next() {
		var v models.LedgerEntry
		if err := rows.Scan(&v.At, &v.Type, &v.Lot, &v.Delta, &v.Note); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Charges(ctx context.Context, accountID string, limit int) ([]models.ChargeRow, error) {
	rows, err := s.db.Query(ctx, `SELECT c.id::text, a.name, c.action_code, c.status, c.idempotency_key, c.amount, c.created_at
		FROM credit_charges c JOIN applications a ON a.id = c.application_id
		WHERE c.account_id=$1 ORDER BY c.created_at DESC, c.id LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ChargeRow
	for rows.Next() {
		var v models.ChargeRow
		if err := rows.Scan(&v.ID, &v.App, &v.Action, &v.Status, &v.Key, &v.Amount, &v.At); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Flows sums the ledger per lot and destination: apps, expired, revoked, plus what is left.
func (s *Store) Flows(ctx context.Context, accountID string) ([]models.Flow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT l.label || ' ' || left(l.id::text, 4),
		       CASE e.type WHEN 'charge' THEN COALESCE(a.name, 'app')
		                   WHEN 'refund' THEN COALESCE(a.name, 'app')
		                   WHEN 'expire' THEN 'expired'
		                   WHEN 'revoke' THEN 'revoked (payment refund)' END,
		       -SUM(e.delta)
		FROM credit_ledger e
		JOIN credit_lots l ON l.id = e.lot_id
		LEFT JOIN credit_charges ch ON ch.id = e.charge_id
		LEFT JOIN applications a ON a.id = ch.application_id
		WHERE e.account_id = $1 AND e.type <> 'grant'
		GROUP BY 1, 2
		UNION ALL
		SELECT l.label || ' ' || left(l.id::text, 4), 'remaining', l.remaining
		FROM credit_lots l WHERE l.account_id = $1
		ORDER BY 1, 2`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Flow
	for rows.Next() {
		var f models.Flow
		if err := rows.Scan(&f.Lot, &f.Dest, &f.Credits); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

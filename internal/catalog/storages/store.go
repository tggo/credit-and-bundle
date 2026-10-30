// Package storages reads the catalog tables in Postgres.
package storages

import (
	"context"
	"time"

	"bundle-credit-test/internal/catalog/models"
	"bundle-credit-test/internal/platform/db"
)

type Store struct{ db db.Q }

func New(q db.Q) *Store { return &Store{db: q} }

func (s *Store) Offering(ctx context.Context, q db.Q, id string) (models.Offering, error) {
	var o models.Offering
	err := q.QueryRow(ctx, `SELECT id::text, code, name, kind, billing_period, price_minor FROM offerings WHERE id::text=$1 OR code=$1`, id).
		Scan(&o.ID, &o.Code, &o.Name, &o.Kind, &o.Period, &o.Price)
	return o, err
}

func (s *Store) Offerings(ctx context.Context) ([]models.Offering, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, code, name, kind, billing_period, price_minor FROM offerings ORDER BY price_minor DESC, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Offering
	for rows.Next() {
		var o models.Offering
		if err := rows.Scan(&o.ID, &o.Code, &o.Name, &o.Kind, &o.Period, &o.Price); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) Items(ctx context.Context, q db.Q, offeringID string) ([]models.Item, error) {
	rows, err := q.Query(ctx, `SELECT oi.id::text, oi.item_type, p.application_id::text, cp.credits, cp.validity_days::bigint, oi.allocation_weight, cp.application_id::text
		FROM offering_items oi
		LEFT JOIN plans p ON p.id = oi.plan_id
		LEFT JOIN credit_packages cp ON cp.id = oi.credit_package_id
		WHERE oi.offering_id = $1 ORDER BY oi.code`, offeringID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Item
	for rows.Next() {
		var it models.Item
		if err := rows.Scan(&it.ID, &it.Type, &it.AppID, &it.Credits, &it.Validity, &it.Weight, &it.PackApp); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Contents lists what each offering includes, in words, keyed by offering id.
func (s *Store) Contents(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.Query(ctx, `SELECT oi.offering_id::text, COALESCE(a.name, cp.credits::text || ' credits' ||
			CASE WHEN o.billing_period = 'month' THEN ' / month' ELSE ' · ' || cp.validity_days || ' days' END)
		FROM offering_items oi JOIN offerings o ON o.id = oi.offering_id
		LEFT JOIN plans p ON p.id = oi.plan_id LEFT JOIN applications a ON a.id = p.application_id
		LEFT JOIN credit_packages cp ON cp.id = oi.credit_package_id ORDER BY oi.offering_id, oi.item_type DESC, oi.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var off, item string
		if err := rows.Scan(&off, &item); err != nil {
			return nil, err
		}
		out[off] = append(out[off], item)
	}
	return out, rows.Err()
}

func (s *Store) Apps(ctx context.Context) ([]models.App, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id::text, a.code, a.name, a.vendor, ap.action_code, ap.credits FROM applications a
		JOIN action_prices ap ON ap.application_id=a.id
		WHERE ap.valid_from = (SELECT max(valid_from) FROM action_prices x WHERE x.application_id=ap.application_id AND x.action_code=ap.action_code)
		ORDER BY a.name, ap.credits DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.App
	for rows.Next() {
		var a models.App
		var act models.Action
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Vendor, &act.Code, &act.Price); err != nil {
			return nil, err
		}
		if n := len(out); n > 0 && out[n-1].ID == a.ID { // one row per action: fold into the app
			out[n-1].Actions = append(out[n-1].Actions, act)
			continue
		}
		a.Actions = []models.Action{act}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActionPrice is the price in credits valid at the given time.
func (s *Store) ActionPrice(ctx context.Context, appID, action string, at time.Time) (int64, error) {
	var price int64
	err := s.db.QueryRow(ctx, `SELECT credits FROM action_prices WHERE application_id=$1 AND action_code=$2 AND valid_from <= $3
		ORDER BY valid_from DESC LIMIT 1`, appID, action, at).Scan(&price)
	return price, err
}

// PeriodDays is how long one payment of the offering covers.
func (s *Store) PeriodDays(ctx context.Context, o models.Offering) int {
	if o.Period == "month" {
		return 30
	}
	days := 365
	_ = s.db.QueryRow(ctx, `SELECT COALESCE(MAX(cp.validity_days),365) FROM offering_items oi
		JOIN credit_packages cp ON cp.id=oi.credit_package_id WHERE oi.offering_id=$1`, o.ID).Scan(&days)
	return days
}

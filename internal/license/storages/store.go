// Package storages keeps licenses in Postgres.
package storages

import (
	"context"
	"time"

	"bundle-credit-test/internal/license/models"
	"bundle-credit-test/internal/platform/db"
	"bundle-credit-test/internal/platform/ids"
)

type Store struct{ db db.Q }

func New(q db.Q) *Store { return &Store{db: q} }

func (s *Store) Upsert(ctx context.Context, q db.Q, g models.Grant) error {
	_, err := q.Exec(ctx, `INSERT INTO licenses (subscription_id, customer_id, offering_item_id, application_id, key, status, due_date)
		VALUES ($1,$2,$3,$4,$5,'active',$6)
		ON CONFLICT (subscription_id, offering_item_id) DO UPDATE SET due_date = EXCLUDED.due_date, status = 'active'`,
		g.SubscriptionID, g.CustomerID, g.OfferingItemID, g.AppID, ids.New("LIC-"), g.Due)
	return err
}

func (s *Store) RevokeSubscription(ctx context.Context, q db.Q, subscriptionID string) error {
	_, err := q.Exec(ctx, `UPDATE licenses SET status='revoked' WHERE subscription_id=$1`, subscriptionID)
	return err
}

func (s *Store) CutSubscription(ctx context.Context, q db.Q, subscriptionID string, until time.Time) error {
	_, err := q.Exec(ctx, `UPDATE licenses SET due_date=$2 WHERE subscription_id=$1 AND due_date > $2`, subscriptionID, until)
	return err
}

func (s *Store) Active(ctx context.Context, customerID, appID string, at time.Time) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM licenses WHERE customer_id=$1 AND application_id=$2 AND status='active' AND due_date > $3)`,
		customerID, appID, at).Scan(&ok)
	return ok, err
}

func (s *Store) ActiveFrom(ctx context.Context, customerID, appID string, at time.Time) string {
	var from *string
	_ = s.db.QueryRow(ctx, `SELECT string_agg(DISTINCT o.name, ' + ') FROM licenses l JOIN subscriptions s ON s.id=l.subscription_id
		JOIN offerings o ON o.id=s.offering_id
		WHERE l.customer_id=$1 AND l.application_id=$2 AND l.status='active' AND l.due_date > $3`, customerID, appID, at).Scan(&from)
	if from == nil {
		return ""
	}
	return *from
}

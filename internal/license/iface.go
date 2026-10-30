// Package license owns licenses: one per (subscription, plan item), active until its due date.
package license

import (
	"context"
	"time"

	"bundle-credit-test/internal/license/models"
	"bundle-credit-test/internal/platform/db"
)

type Store interface {
	// Upsert issues the license or extends it for a renewal; idempotent by (subscription, item).
	Upsert(ctx context.Context, q db.Q, g models.Grant) error
	RevokeSubscription(ctx context.Context, q db.Q, subscriptionID string) error
	CutSubscription(ctx context.Context, q db.Q, subscriptionID string, until time.Time) error
	Active(ctx context.Context, customerID, appID string, at time.Time) (bool, error)
	// ActiveFrom names the offerings that give the customer this app now ("" when none).
	ActiveFrom(ctx context.Context, customerID, appID string, at time.Time) string
}

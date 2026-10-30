// Package catalog owns applications, plans, credit packages, offerings and AI action prices.
package catalog

import (
	"context"
	"time"

	"bundle-credit-test/internal/catalog/models"
	"bundle-credit-test/internal/platform/db"
)

// Store reads the catalog. q is the caller's pool or transaction.
type Store interface {
	Offering(ctx context.Context, q db.Q, id string) (models.Offering, error)
	Offerings(ctx context.Context) ([]models.Offering, error)
	Items(ctx context.Context, q db.Q, offeringID string) ([]models.Item, error)
	Contents(ctx context.Context) (map[string][]string, error)
	Apps(ctx context.Context) ([]models.App, error)
	ActionPrice(ctx context.Context, appID, action string, at time.Time) (int64, error)
	PeriodDays(ctx context.Context, o models.Offering) int
}

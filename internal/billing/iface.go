// Package billing simulates the Billing team's side: provider checkout, webhooks, renewals, refunds, reconciliation.
// It hands paid periods to License and Credits through their interfaces, inside its own transaction.
package billing

import (
	"context"
	"time"

	"bundle-credit-test/internal/billing/models"
	catmodels "bundle-credit-test/internal/catalog/models"
	credmodels "bundle-credit-test/internal/credits/models"
	licmodels "bundle-credit-test/internal/license/models"
	"bundle-credit-test/internal/platform/db"
)

type Store interface {
	RecordEvent(ctx context.Context, q db.Q, ev models.Event, at time.Time) (fresh bool, err error)
	SetEventResult(ctx context.Context, q db.Q, eventID, status, reason string) error
	TakeParked(ctx context.Context, subRef string) ([]models.Event, error)

	LockSubscription(ctx context.Context, q db.Q, subRef string) (*models.Subscription, error)
	InsertSubscription(ctx context.Context, q db.Q, ev models.Event, kind string, at time.Time) error
	SetSubscriptionStatus(ctx context.Context, q db.Q, id, status string) error
	ActivatePeriod(ctx context.Context, q db.Q, id string, start, end time.Time) error
	Subscription(ctx context.Context, id string) (models.Subscription, error)
	LatestSubscription(ctx context.Context, customerID string, activeRecurring bool) (models.Subscription, error)
	DueRenewals(ctx context.Context, at time.Time) ([]models.Subscription, error)

	PaymentExists(ctx context.Context, q db.Q, ref string) (bool, error)
	InsertPayment(ctx context.Context, q db.Q, subID string, ev models.Event, at time.Time) (string, error)
	LockPayment(ctx context.Context, q db.Q, ref string) (id, subID, status string, err error)
	RefundPayment(ctx context.Context, q db.Q, id string, amount int64) error
	PaidUntil(ctx context.Context, q db.Q, subscriptionID string) (*time.Time, error)
	LatestPaid(ctx context.Context, subscriptionID string) (models.Paid, error)
	LatestCustomerPayment(ctx context.Context, customerID string) (models.Paid, string, error)
	PeriodStartBefore(ctx context.Context, subRef string, before time.Time) (time.Time, error)
	InjectPhantom(ctx context.Context, ref string, at time.Time) error

	ProviderCharge(ctx context.Context, ref, subRef, customerID, offeringID string, amount int64, at time.Time) error
	ProviderRefund(ctx context.Context, ref string, amount int64) error
	ProviderTxn(ctx context.Context, ref string) (models.ProviderTxn, error)
	ProviderSilentRefund(ctx context.Context) (string, error)
	PaymentStatus(ctx context.Context, ref string) (*string, error)
	Reconcile(ctx context.Context) ([]models.ReconRow, int, error)
	NetTotals(ctx context.Context) (provider, ours int64)

	SubViews(ctx context.Context, customerID string) ([]models.SubView, error)
	Webhooks(ctx context.Context, limit int) ([]models.WebhookRow, error)
}

// Catalog is what billing reads about offerings.
type Catalog interface {
	Offering(ctx context.Context, q db.Q, id string) (catmodels.Offering, error)
	Items(ctx context.Context, q db.Q, offeringID string) ([]catmodels.Item, error)
	PeriodDays(ctx context.Context, o catmodels.Offering) int
}

// Licenses issues and revokes licenses inside billing's transaction.
type Licenses interface {
	Upsert(ctx context.Context, q db.Q, g licmodels.Grant) error
	RevokeSubscription(ctx context.Context, q db.Q, subscriptionID string) error
	// CutSubscription ends the licenses at until (refund of a later period; an earlier one is still paid).
	CutSubscription(ctx context.Context, q db.Q, subscriptionID string, until time.Time) error
}

// Credits grants and revokes lots inside billing's transaction.
type Credits interface {
	GrantLot(ctx context.Context, q db.Q, g credmodels.Grant) error
	RevokePayment(ctx context.Context, q db.Q, paymentID string) error
	PaymentUsage(ctx context.Context, paymentID string) (credmodels.Usage, error)
	Expire(ctx context.Context) error
}

type Clock interface {
	Now(ctx context.Context) time.Time
	Advance(ctx context.Context, days int) error
}

type Log interface {
	Log(kind, format string, args ...any)
	Changed()
}

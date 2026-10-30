// Package credits owns wallets, lots, charges and the append-only ledger.
package credits

import (
	"context"
	"time"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

// Store is the credits storage the service needs.
type Store interface {
	Account(ctx context.Context, q db.Q, customerID string) (id, status string, err error)
	LockAccount(ctx context.Context, q db.Q, accountID string) error
	LockChargeAccount(ctx context.Context, q db.Q, chargeID string) error
	LockPaymentAccounts(ctx context.Context, q db.Q, paymentID string) error
	LockExpiredAccounts(ctx context.Context, q db.Q, at time.Time) error
	Available(ctx context.Context, accountID, appID string, at time.Time) int64

	InsertCharge(ctx context.Context, q db.Q, accountID string, r models.ChargeRequest, price int64, reqHash string, at time.Time) (id string, inserted bool, err error)
	StoredCharge(ctx context.Context, accountID, key string) (id, status, hash string, amount int64, result []byte, err error)
	CompleteCharge(ctx context.Context, chargeID string, result []byte, cost int64) (bool, error)
	ClaimRefund(ctx context.Context, q db.Q, chargeID string) (bool, error)
	StaleCharges(ctx context.Context, before time.Time) ([]string, error)

	EligibleLots(ctx context.Context, q db.Q, accountID, appID string, at time.Time) ([]models.LotLeft, error)
	Debit(ctx context.Context, q db.Q, lotID string, amount int64) (ok bool, err error)
	Credit(ctx context.Context, q db.Q, lotID string, amount int64) error
	LotState(ctx context.Context, q db.Q, lotID string) (revoked bool, expires time.Time, err error)
	Write(ctx context.Context, q db.Q, e models.Entry) error
	ChargeEntries(ctx context.Context, q db.Q, chargeID string) ([]models.Entry, error)

	InsertLot(ctx context.Context, q db.Q, accountID string, g models.Grant) (id string, inserted bool, err error)
	ZeroPaymentLots(ctx context.Context, q db.Q, paymentID string, at time.Time) ([]models.LotLeft, error)
	ZeroExpiredLots(ctx context.Context, q db.Q, at time.Time) ([]models.LotLeft, error)
	PaymentUsage(ctx context.Context, paymentID string) (models.Usage, error)

	Balance(ctx context.Context, accountID, appID string, at time.Time) (models.Balance, error)
	Lots(ctx context.Context, accountID string, at time.Time) ([]models.Lot, error)
	Ledger(ctx context.Context, accountID string, limit int) ([]models.LedgerEntry, error)
	Charges(ctx context.Context, accountID string, limit int) ([]models.ChargeRow, error)
	Flows(ctx context.Context, accountID string) ([]models.Flow, error)
}

// Licenses tells whether the customer may use the app now.
type Licenses interface {
	Active(ctx context.Context, customerID, appID string, at time.Time) (bool, error)
}

// Prices gives the price of an AI action.
type Prices interface {
	ActionPrice(ctx context.Context, appID, action string, at time.Time) (int64, error)
}

// AI is the provider behind the gateway. blocked: content policy refused, still charged.
type AI interface {
	Run(ctx context.Context, action, input string) (result []byte, blocked bool, err error)
}

type Clock interface {
	Now(ctx context.Context) time.Time
}

type Log interface {
	Log(kind, format string, args ...any)
}

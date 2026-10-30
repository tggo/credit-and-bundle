package services

import (
	"context"

	"bundle-credit-test/internal/credits/models"
)

// Balance is what an app sees: available here, total and the lots behind it. appID "" means the whole wallet.
func (s *Service) Balance(ctx context.Context, customerID, appID string) (models.Balance, error) {
	acc, _, err := s.store.Account(ctx, s.pool, customerID)
	if err != nil {
		return models.Balance{}, err
	}
	return s.store.Balance(ctx, acc, appID, s.clock.Now(ctx))
}

func (s *Service) Available(ctx context.Context, customerID, appID string) int64 {
	acc, _, err := s.store.Account(ctx, s.pool, customerID)
	if err != nil {
		return 0
	}
	return s.store.Available(ctx, acc, appID, s.clock.Now(ctx))
}

// Wallet is everything the simulator shows about one customer's credits.
type Wallet struct {
	Status  string
	Lots    []models.Lot
	Ledger  []models.LedgerEntry
	Charges []models.ChargeRow
}

func (s *Service) Wallet(ctx context.Context, customerID string) (Wallet, error) {
	acc, status, err := s.store.Account(ctx, s.pool, customerID)
	if err != nil {
		return Wallet{}, err
	}
	w := Wallet{Status: status}
	if w.Lots, err = s.store.Lots(ctx, acc, s.clock.Now(ctx)); err != nil {
		return w, err
	}
	if w.Ledger, err = s.store.Ledger(ctx, acc, 25); err != nil {
		return w, err
	}
	w.Charges, err = s.store.Charges(ctx, acc, 12)
	return w, err
}

func (s *Service) Flows(ctx context.Context, customerID string) ([]models.Flow, error) {
	acc, _, err := s.store.Account(ctx, s.pool, customerID)
	if err != nil {
		return nil, err
	}
	return s.store.Flows(ctx, acc)
}

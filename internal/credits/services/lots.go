package services

import (
	"context"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

// GrantLot creates the lot for a paid credit item inside the caller's transaction. Idempotent per (payment, item).
func (s *Service) GrantLot(ctx context.Context, q db.Q, g models.Grant) error {
	acc, _, err := s.store.Account(ctx, q, g.CustomerID)
	if err != nil {
		return err
	}
	id, inserted, err := s.store.InsertLot(ctx, q, acc, g)
	if err != nil || !inserted {
		return err
	}
	return s.store.Write(ctx, q, models.Entry{AccountID: acc, LotID: id, Type: "grant", Delta: g.Credits, Note: g.Label, At: g.At})
}

// RevokePayment takes back the unspent rest of the lots this payment granted. Lots of other periods of the same
// subscription stay: they were paid by other payments.
func (s *Service) RevokePayment(ctx context.Context, q db.Q, paymentID string) error {
	now := s.clock.Now(ctx)
	if err := s.store.LockPaymentAccounts(ctx, q, paymentID); err != nil { // lock order as in a charge
		return err
	}
	lots, err := s.store.ZeroPaymentLots(ctx, q, paymentID, now)
	if err != nil {
		return err
	}
	for _, l := range lots {
		if l.Remaining == 0 { // marked revoked, nothing to take back now
			continue
		}
		if err := s.store.Write(ctx, q, models.Entry{AccountID: l.AccountID, LotID: l.ID, Type: "revoke",
			Delta: -l.Remaining, Note: "payment refunded", At: now}); err != nil {
			return err
		}
	}
	return nil
}

// Expire burns the rest of every lot past its expiry: the liability becomes revenue.
func (s *Service) Expire(ctx context.Context) error {
	now := s.clock.Now(ctx)
	return db.InTx(ctx, s.pool, func(q db.Q) error {
		if err := s.store.LockExpiredAccounts(ctx, q, now); err != nil { // lock order as in a charge
			return err
		}
		lots, err := s.store.ZeroExpiredLots(ctx, q, now)
		if err != nil {
			return err
		}
		for _, l := range lots {
			if err := s.store.Write(ctx, q, models.Entry{AccountID: l.AccountID, LotID: l.ID, Type: "expire",
				Delta: -l.Remaining, Note: "expired", At: now}); err != nil {
				return err
			}
			s.log.Log("warn", "lot %q expired: %d credits burned", l.Label, l.Remaining)
		}
		return nil
	})
}

func (s *Service) PaymentUsage(ctx context.Context, paymentID string) (models.Usage, error) {
	return s.store.PaymentUsage(ctx, paymentID)
}

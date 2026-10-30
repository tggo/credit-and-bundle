package services

import (
	"context"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

// RefundCharge returns a charge's credits to the same lots they were taken from. A lot whose payment was refunded
// or that expired meanwhile gets them back and loses them again at once: credits never outlive their lot.
func (s *Service) RefundCharge(ctx context.Context, chargeID, why string) error {
	now := s.clock.Now(ctx)
	return db.InTx(ctx, s.pool, func(q db.Q) error {
		// Account first, as in a charge: the ledger insert below takes a key-share lock on it.
		if err := s.store.LockChargeAccount(ctx, q, chargeID); err != nil {
			return err
		}
		// Claim first: the request and the sweeper may both try; only the one that flips in_progress returns credits.
		claimed, err := s.store.ClaimRefund(ctx, q, chargeID)
		if err != nil || !claimed {
			return err
		}
		entries, err := s.store.ChargeEntries(ctx, q, chargeID)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := s.store.Credit(ctx, q, e.LotID, e.Delta); err != nil {
				return err
			}
			if err := s.store.Write(ctx, q, models.Entry{AccountID: e.AccountID, LotID: e.LotID, ChargeID: chargeID,
				Type: "refund", Delta: e.Delta, Note: why, At: now}); err != nil {
				return err
			}
			if err := s.burnIfDead(ctx, q, e, now); err != nil {
				return err
			}
		}
		return nil
	})
}

package services

import (
	"context"
	"time"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
)

// burnIfDead takes refunded credits back out of a lot that was revoked or expired while the charge ran.
func (s *Service) burnIfDead(ctx context.Context, q db.Q, e models.Entry, now time.Time) error {
	revoked, expires, err := s.store.LotState(ctx, q, e.LotID)
	if err != nil {
		return err
	}
	typ, note := "revoke", "payment refunded"
	switch {
	case revoked:
	case !expires.After(now):
		typ, note = "expire", "expired"
	default:
		return nil
	}
	if _, err := s.store.Debit(ctx, q, e.LotID, e.Delta); err != nil {
		return err
	}
	return s.store.Write(ctx, q, models.Entry{AccountID: e.AccountID, LotID: e.LotID, Type: typ, Delta: -e.Delta, Note: note, At: now})
}

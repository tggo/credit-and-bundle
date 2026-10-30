package services

import (
	"context"

	"bundle-credit-test/internal/billing/models"
)

func (s *Service) Reconcile(ctx context.Context) ([]models.ReconRow, int, error) {
	return s.store.Reconcile(ctx)
}

func (s *Service) NetTotals(ctx context.Context) (int64, int64) { return s.store.NetTotals(ctx) }

// FixRecon replays the missing webhook from the provider's record.
func (s *Service) FixRecon(ctx context.Context, ref string) error {
	t, err := s.store.ProviderTxn(ctx, ref)
	if err != nil {
		return err
	}
	ours, _ := s.store.PaymentStatus(ctx, ref)
	if ours != nil && t.Status == "refunded" {
		s.Deliver(ctx, models.Event{EventID: "evt_" + ref + "_refunded_replay", Type: "payment.refunded", SubRef: t.SubRef,
			PaymentRef: ref, CustomerID: t.CustomerID, OfferingID: t.OfferingID, RefundMinor: t.Refunded})
		return nil
	}
	o, err := s.catalog.Offering(ctx, s.pool, t.OfferingID)
	if err != nil {
		return err
	}
	start, err := s.store.PeriodStartBefore(ctx, t.SubRef, t.Created)
	if err != nil {
		return err
	}
	s.Deliver(ctx, models.Event{EventID: "evt_" + ref + "_paid_replay", Type: "invoice.paid", SubRef: t.SubRef, PaymentRef: ref,
		CustomerID: t.CustomerID, OfferingID: o.ID, AmountMinor: o.Price, PeriodStart: start,
		PeriodEnd: start.AddDate(0, 0, s.catalog.PeriodDays(ctx, o))})
	return nil
}

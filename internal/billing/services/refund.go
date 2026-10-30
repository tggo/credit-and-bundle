package services

import (
	"context"
	"fmt"
	"math"

	"bundle-credit-test/internal/billing/models"
)

// Quote is the partial refund of the latest payment: apps part by unused time, credits part by unspent share.
// The split of the price comes from the items' allocation weights.
func (s *Service) Quote(ctx context.Context, subscriptionID string) (models.RefundQuote, error) {
	var q models.RefundQuote
	p, err := s.store.LatestPaid(ctx, subscriptionID)
	if err != nil {
		return q, fmt.Errorf("no refundable payment: %w", err)
	}
	items, err := s.catalog.Items(ctx, s.pool, p.OfferingID)
	if err != nil {
		return q, err
	}
	var appsW, total int64
	for _, it := range items {
		total += it.Weight
		if it.Type == "plan" {
			appsW += it.Weight
		}
	}
	q.PaymentRef, q.Amount = p.Ref, p.Amount
	if total > 0 {
		q.AppsPart = p.Amount * appsW / total
	}
	q.CredPart = p.Amount - q.AppsPart
	now := s.clock.Now(ctx)
	q.UnusedFrac = math.Max(0, math.Min(1, p.End.Sub(now).Seconds()/p.End.Sub(p.Start).Seconds()))
	if u, err := s.credits.PaymentUsage(ctx, p.ID); err == nil && u.Granted > 0 {
		q.CredFrac = float64(u.Remaining) / float64(u.Granted)
	}
	q.Refund = int64(math.Round(float64(q.AppsPart)*q.UnusedFrac + float64(q.CredPart)*q.CredFrac))
	return q, nil
}

// Refund refunds the quoted amount at the provider; its payment.refunded webhook revokes rights.
func (s *Service) Refund(ctx context.Context, subscriptionID string) error {
	q, err := s.Quote(ctx, subscriptionID)
	if err != nil {
		return err
	}
	s.log.Log("info", "refund quote %s: apps $%.2f × %.0f%% unused + credits $%.2f × %.0f%% unspent = $%.2f",
		q.PaymentRef, float64(q.AppsPart)/100, q.UnusedFrac*100, float64(q.CredPart)/100, q.CredFrac*100, float64(q.Refund)/100)
	sub, err := s.store.Subscription(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if err := s.store.ProviderRefund(ctx, q.PaymentRef, q.Refund); err != nil {
		return err
	}
	s.Deliver(ctx, models.Event{EventID: "evt_" + q.PaymentRef + "_refunded", Type: "payment.refunded", SubRef: sub.Ref,
		PaymentRef: q.PaymentRef, CustomerID: sub.CustomerID, OfferingID: sub.OfferingID, RefundMinor: q.Refund})
	return nil
}

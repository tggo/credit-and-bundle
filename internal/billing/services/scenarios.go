package services

import (
	"context"
	"fmt"

	"bundle-credit-test/internal/billing/models"
	"bundle-credit-test/internal/platform/ids"
)

// Scenarios for the webhook and reconciliation pages.

// DuplicatePaid re-sends the customer's latest invoice.paid: same event_id, then a new event_id with the same payment_ref.
func (s *Service) DuplicatePaid(ctx context.Context, customerID string) error {
	p, subRef, err := s.store.LatestCustomerPayment(ctx, customerID)
	if err != nil {
		return fmt.Errorf("no payments yet — buy something first")
	}
	ev := models.Event{EventID: "evt_" + p.Ref + "_paid", Type: "invoice.paid", SubRef: subRef, PaymentRef: p.Ref,
		CustomerID: customerID, OfferingID: p.OfferingID, AmountMinor: p.Amount, PeriodStart: p.Start, PeriodEnd: p.End}
	s.log.Log("info", "scenario: provider re-sends the same invoice.paid")
	s.Deliver(ctx, ev)
	ev.EventID = ids.New("evt_resent_")
	s.log.Log("info", "scenario: same payment_ref under a new event_id")
	s.Deliver(ctx, ev)
	return nil
}

// LateCreated cancels the latest subscription, then a delayed subscription.created arrives.
func (s *Service) LateCreated(ctx context.Context, customerID string) error {
	sub, err := s.store.LatestSubscription(ctx, customerID, false)
	if err != nil {
		return fmt.Errorf("no subscriptions yet")
	}
	if sub.Status != "canceled" {
		if err := s.Cancel(ctx, sub.ID); err != nil {
			return err
		}
	}
	s.log.Log("info", "scenario: a delayed subscription.created arrives after cancel")
	s.Deliver(ctx, models.Event{EventID: ids.New("evt_late_created_"), Type: "subscription.created", SubRef: sub.Ref,
		CustomerID: customerID, OfferingID: sub.OfferingID})
	return nil
}

// PastDue declines a renewal, then the retry is paid.
func (s *Service) PastDue(ctx context.Context, customerID string) error {
	sub, err := s.store.LatestSubscription(ctx, customerID, true)
	if err != nil {
		return fmt.Errorf("no active recurring subscription")
	}
	s.log.Log("info", "scenario: renewal card declined, then paid on retry")
	s.Deliver(ctx, models.Event{EventID: ids.New("evt_failed_"), Type: "invoice.failed", SubRef: sub.Ref, CustomerID: customerID, OfferingID: sub.OfferingID})
	o, err := s.catalog.Offering(ctx, s.pool, sub.OfferingID)
	if err != nil {
		return err
	}
	ev, err := s.charge(ctx, sub.Ref, customerID, o, *sub.PeriodEnd)
	if err != nil {
		return err
	}
	s.Deliver(ctx, ev)
	return nil
}

// Phantom records a payment the provider does not know (a bug on our side).
func (s *Service) Phantom(ctx context.Context) error {
	ref := ids.New("pay_phantom_")
	if err := s.store.InjectPhantom(ctx, ref, s.clock.Now(ctx)); err != nil {
		return err
	}
	s.log.Log("err", "bug injected: payment %s recorded on our side only", ref)
	return nil
}

// SilentRefund refunds at the provider without a webhook.
func (s *Service) SilentRefund(ctx context.Context) error {
	ref, err := s.store.ProviderSilentRefund(ctx)
	if err != nil {
		return fmt.Errorf("no provider transactions yet")
	}
	s.log.Log("err", "provider refunded %s in its dashboard, no webhook sent", ref)
	return nil
}

// Views for the pages.

func (s *Service) Subscriptions(ctx context.Context, customerID string) ([]models.SubView, error) {
	subs, err := s.store.SubViews(ctx, customerID)
	if err != nil || customerID == "" {
		return subs, err
	}
	for i := range subs {
		if subs[i].Status != "canceled" && subs[i].End != nil {
			if q, err := s.Quote(ctx, subs[i].ID); err == nil {
				subs[i].Quote = &q
			}
		}
	}
	return subs, nil
}

func (s *Service) Webhooks(ctx context.Context) ([]models.WebhookRow, error) {
	return s.store.Webhooks(ctx, 60)
}

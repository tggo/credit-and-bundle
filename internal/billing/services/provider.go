package services

import (
	"context"
	"time"

	"bundle-credit-test/internal/billing/models"
	catmodels "bundle-credit-test/internal/catalog/models"
	"bundle-credit-test/internal/platform/faults"
	"bundle-credit-test/internal/platform/ids"
)

// charge records a successful payment at the mock provider and returns its invoice.paid event.
func (s *Service) charge(ctx context.Context, subRef, customerID string, o catmodels.Offering, start time.Time) (models.Event, error) {
	ref := ids.New("pay_")
	end := start.AddDate(0, 0, s.catalog.PeriodDays(ctx, o))
	err := s.store.ProviderCharge(ctx, ref, subRef, customerID, o.ID, o.Price, s.clock.Now(ctx))
	return models.Event{EventID: "evt_" + ref + "_paid", Type: "invoice.paid", SubRef: subRef, PaymentRef: ref,
		CustomerID: customerID, OfferingID: o.ID, AmountMinor: o.Price, PeriodStart: start, PeriodEnd: end}, err
}

// Purchase simulates checkout at the provider followed by its webhooks. outOfOrder sends invoice.paid first.
func (s *Service) Purchase(ctx context.Context, customerID, handle, offeringID string, outOfOrder bool) error {
	o, err := s.catalog.Offering(ctx, s.pool, offeringID)
	if err != nil {
		return err
	}
	subRef := ids.New("sub_")
	s.log.Log("info", "%s buys %s ($%.2f) → provider %s", handle, o.Name, float64(o.Price)/100, subRef)
	created := models.Event{EventID: "evt_" + subRef + "_created", Type: "subscription.created", SubRef: subRef, CustomerID: customerID, OfferingID: o.ID}
	paid, err := s.charge(ctx, subRef, customerID, o, s.clock.Now(ctx))
	if err != nil {
		return err
	}
	events := []models.Event{created, paid}
	if outOfOrder {
		events = []models.Event{paid, created}
	}
	for _, ev := range events {
		if ev.Type == "invoice.paid" && faults.Take(&s.faults.LoseWebhook) {
			s.log.Log("err", "provider charged %s but webhook %s was LOST", ev.PaymentRef, ev.EventID)
			continue
		}
		s.Deliver(ctx, ev)
	}
	return nil
}

// Advance moves the clock, charges due renewals and expires lots.
func (s *Service) Advance(ctx context.Context, days int) error {
	if err := s.clock.Advance(ctx, days); err != nil {
		return err
	}
	s.log.Log("info", "clock +%d day(s) → %s", days, s.clock.Now(ctx).Format("2006-01-02 15:04"))
	if err := s.renewDue(ctx); err != nil {
		return err
	}
	return s.credits.Expire(ctx)
}

// renewDue charges every active recurring subscription whose period has ended, catching up at most 24 periods.
func (s *Service) renewDue(ctx context.Context) error {
	for i := 0; i < 24; i++ {
		due, err := s.store.DueRenewals(ctx, s.clock.Now(ctx))
		if err != nil || len(due) == 0 {
			return err
		}
		for _, sub := range due {
			o, err := s.catalog.Offering(ctx, s.pool, sub.OfferingID)
			if err != nil {
				return err
			}
			ev, err := s.charge(ctx, sub.Ref, sub.CustomerID, o, *sub.PeriodEnd)
			if err != nil {
				return err
			}
			s.log.Log("info", "renewal: provider charged %s for %s (next period from %s)", sub.Ref, o.Name, sub.PeriodEnd.Format("Jan 2"))
			s.Deliver(ctx, ev)
		}
	}
	return nil
}

// Cancel asks the provider to cancel at period end; its webhook does the rest.
func (s *Service) Cancel(ctx context.Context, subscriptionID string) error {
	sub, err := s.store.Subscription(ctx, subscriptionID)
	if err != nil {
		return err
	}
	s.Deliver(ctx, models.Event{EventID: ids.New("evt_cancel_"), Type: "subscription.canceled", SubRef: sub.Ref,
		CustomerID: sub.CustomerID, OfferingID: sub.OfferingID})
	return nil
}

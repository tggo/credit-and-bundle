package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"bundle-credit-test/internal/billing/models"
	"bundle-credit-test/internal/platform/db"
)

// apply moves the subscription by the allowed transitions only; a terminal state never comes back.
func (s *Service) apply(ctx context.Context, q db.Q, ev models.Event, now time.Time) (string, string, error) {
	switch ev.Type {
	case "subscription.created":
		return s.onCreated(ctx, q, ev, now)
	case "invoice.paid":
		return s.onPaid(ctx, q, ev, now)
	case "invoice.failed":
		return s.move(ctx, q, ev, "past_due")
	case "subscription.canceled":
		// Cancel at period end: licenses and lots live until their own dates.
		return s.move(ctx, q, ev, "canceled")
	case "payment.refunded":
		return s.onRefunded(ctx, q, ev)
	}
	return "rejected", "unknown event type", nil
}

func (s *Service) onCreated(ctx context.Context, q db.Q, ev models.Event, now time.Time) (string, string, error) {
	sub, err := s.store.LockSubscription(ctx, q, ev.SubRef)
	if err != nil {
		return "", "", err
	}
	if sub != nil {
		return "rejected", fmt.Sprintf("subscription already exists in state %q; a late 'created' must not revive it", sub.Status), nil
	}
	o, err := s.catalog.Offering(ctx, q, ev.OfferingID)
	if err != nil {
		return "", "", err
	}
	kind := "recurring"
	if o.Period == "one_time" {
		kind = "one_time"
	}
	return "applied", "", s.store.InsertSubscription(ctx, q, ev, kind, now)
}

func (s *Service) onPaid(ctx context.Context, q db.Q, ev models.Event, now time.Time) (string, string, error) {
	sub, err := s.store.LockSubscription(ctx, q, ev.SubRef)
	if err != nil {
		return "", "", err
	}
	if sub == nil {
		return "parked", "subscription not known yet; will retry after subscription.created", nil
	}
	dup, err := s.store.PaymentExists(ctx, q, ev.PaymentRef)
	if err != nil {
		return "", "", err
	}
	if dup {
		return "duplicate", "payment_ref already recorded", nil
	}
	if !models.Transitions[sub.Status]["active"] {
		return "rejected", fmt.Sprintf("transition %s → active is not allowed", sub.Status), nil
	}
	payID, err := s.store.InsertPayment(ctx, q, sub.ID, ev, now)
	if err != nil {
		return "", "", err
	}
	if err := s.store.ActivatePeriod(ctx, q, sub.ID, ev.PeriodStart, ev.PeriodEnd); err != nil {
		return "", "", err
	}
	return "applied", "", s.grant(ctx, q, sub, payID, ev, now)
}

func (s *Service) move(ctx context.Context, q db.Q, ev models.Event, to string) (string, string, error) {
	sub, err := s.store.LockSubscription(ctx, q, ev.SubRef)
	if err != nil || sub == nil {
		return "rejected", "unknown subscription", err
	}
	if !models.Transitions[sub.Status][to] {
		return "rejected", fmt.Sprintf("transition %s → %s is not allowed", sub.Status, to), nil
	}
	return "applied", "", s.store.SetSubscriptionStatus(ctx, q, sub.ID, to)
}

func (s *Service) onRefunded(ctx context.Context, q db.Q, ev models.Event) (string, string, error) {
	payID, subID, status, err := s.store.LockPayment(ctx, q, ev.PaymentRef)
	if errors.Is(err, pgx.ErrNoRows) {
		// The refund overtook its invoice.paid: keep it and apply it once that payment is recorded.
		return "parked", "payment not known yet; will retry after invoice.paid", nil
	}
	if err != nil {
		return "", "", err
	}
	if status != "succeeded" {
		return "rejected", fmt.Sprintf("payment is %s, refund is terminal", status), nil
	}
	if err := s.store.RefundPayment(ctx, q, payID, ev.RefundMinor); err != nil {
		return "", "", err
	}
	if err := s.store.SetSubscriptionStatus(ctx, q, subID, "canceled"); err != nil {
		return "", "", err
	}
	// Only what this payment bought goes back: its lots, and access for its period. An earlier period that is
	// still paid (e.g. the one before a past_due renewal) keeps its lots and runs to its own end.
	until, err := s.store.PaidUntil(ctx, q, subID)
	if err != nil {
		return "", "", err
	}
	if until != nil && until.After(s.clock.Now(ctx)) {
		err = s.lic.CutSubscription(ctx, q, subID, *until)
	} else {
		err = s.lic.RevokeSubscription(ctx, q, subID)
	}
	if err != nil {
		return "", "", err
	}
	return "applied", "", s.credits.RevokePayment(ctx, q, payID)
}

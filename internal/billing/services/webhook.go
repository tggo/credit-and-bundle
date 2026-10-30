package services

import (
	"context"

	"bundle-credit-test/internal/billing/models"
	"bundle-credit-test/internal/platform/db"
)

// Deliver runs one provider event through the inbox and logs the outcome.
func (s *Service) Deliver(ctx context.Context, ev models.Event) {
	status, reason, err := s.Handle(ctx, ev)
	switch {
	case err != nil:
		s.log.Log("err", "webhook %s %s: error %v", ev.Type, ev.EventID, err)
	case status == "applied":
		s.log.Log("ok", "webhook %s %s → applied", ev.Type, ev.EventID)
	case status == "parked":
		s.log.Log("warn", "webhook %s %s → parked: %s", ev.Type, ev.EventID, reason)
	default:
		s.log.Log("warn", "webhook %s %s → %s: %s", ev.Type, ev.EventID, status, reason)
	}
	if status == "applied" && (ev.Type == "subscription.created" || ev.Type == "invoice.paid") {
		s.retryParked(ctx, ev.SubRef)
	}
	s.log.Changed()
}

// Handle records the event (dedup by event_id) and applies it in one transaction.
func (s *Service) Handle(ctx context.Context, ev models.Event) (status, reason string, err error) {
	now := s.clock.Now(ctx)
	err = db.InTx(ctx, s.pool, func(q db.Q) error {
		fresh, err := s.store.RecordEvent(ctx, q, ev, now)
		if err != nil {
			return err
		}
		if !fresh {
			status, reason = "duplicate", "event_id already processed"
			return nil
		}
		if status, reason, err = s.apply(ctx, q, ev, now); err != nil {
			return err
		}
		return s.store.SetEventResult(ctx, q, ev.EventID, status, reason)
	})
	return status, reason, err
}

func (s *Service) retryParked(ctx context.Context, subRef string) {
	evs, err := s.store.TakeParked(ctx, subRef)
	if err != nil {
		s.log.Log("err", "retry parked: %v", err)
		return
	}
	for _, ev := range evs {
		s.log.Log("info", "retrying parked %s %s", ev.Type, ev.EventID)
		s.Deliver(ctx, ev)
	}
}

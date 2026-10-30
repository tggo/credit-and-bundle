package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"bundle-credit-test/internal/credits/models"
	"bundle-credit-test/internal/platform/db"
	"bundle-credit-test/internal/platform/faults"
)

// ChargeTimeout is how long a charge may stay in_progress. Longer than any AI call (provider timeout 60 s),
// so a live request is never swept; after it the request is treated as dead and its credits go back.
const ChargeTimeout = 2 * time.Minute

// ProviderTimeout bounds the AI call; a timeout is a provider failure and returns the credits.
const ProviderTimeout = 60 * time.Second

// SweepStale refunds charges stuck in_progress past ChargeTimeout (process died between debit and complete).
// Safe to run on every instance at once: ClaimRefund lets exactly one of them return the credits.
func (s *Service) SweepStale(ctx context.Context) (int, error) {
	ids, err := s.store.StaleCharges(ctx, s.clock.Now(ctx).Add(-ChargeTimeout))
	if err != nil {
		return 0, err
	}
	var firstErr error
	for _, id := range ids { // one failed refund must not hold back the others
		if err := s.RefundCharge(ctx, id, "stale in_progress (request died)"); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return 0, firstErr
	}
	if len(ids) > 0 {
		// A refund may land in a lot that expired while the request hung: burn it now, not at the next pass.
		if err := s.Expire(ctx); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// Charge implements POST /api/v1/credits/charges: checks, one debit transaction, the AI call, then complete or refund.
// Returns the HTTP status and body.
func (s *Service) Charge(ctx context.Context, r models.ChargeRequest) (int, models.ChargeResult) {
	now := s.clock.Now(ctx)
	acc, status, err := s.store.Account(ctx, s.pool, r.CustomerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 404, models.Fail("unknown_customer")
	}
	if err != nil {
		return 500, models.Fail(err.Error())
	}
	if status == "blocked" {
		return 403, models.Fail("account_blocked")
	}
	// A known key answers from the stored charge before any other check: a retry must get the same answer
	// even if the license or the price changed since the first attempt.
	if _, _, _, _, _, err := s.store.StoredCharge(ctx, acc, r.Key); err == nil {
		sum := sha256.Sum256([]byte(r.AppID + "|" + r.Action + "|" + r.Input))
		return s.replay(ctx, acc, r, hex.EncodeToString(sum[:8]))
	}
	if ok, err := s.lic.Active(ctx, r.CustomerID, r.AppID, now); err != nil {
		return 500, models.Fail(err.Error())
	} else if !ok {
		return 403, models.Fail("no_active_license")
	}
	price, err := s.prices.ActionPrice(ctx, r.AppID, r.Action, now)
	if err != nil {
		return 404, models.Fail("unknown_action")
	}
	sum := sha256.Sum256([]byte(r.AppID + "|" + r.Action + "|" + r.Input))
	hash := hex.EncodeToString(sum[:8])

	chargeID, code, res := s.debit(ctx, acc, r, price, hash)
	if chargeID == "" {
		return code, res
	}

	// Outside the transaction: the provider call. Our-side failure returns the credits.
	aiCtx, cancel := context.WithTimeout(ctx, ProviderTimeout)
	result, blocked, err := s.ai.Run(aiCtx, r.Action, r.Input)
	cancel()
	if errors.Is(err, faults.ErrCrash) {
		// The process "dies" here: no refund, the charge stays in_progress until SweepStale returns the credits.
		return 500, models.ChargeResult{ChargeID: chargeID, Status: "in_progress", Error: "simulated_crash"}
	}
	if err != nil {
		if err := s.RefundCharge(ctx, chargeID, "provider 5xx"); err != nil {
			return 500, models.Fail(err.Error())
		}
		return 502, models.ChargeResult{ChargeID: chargeID, Status: "refunded", Error: "upstream_failed", Refunded: true,
			Available: s.store.Available(ctx, acc, r.AppID, now)}
	}
	done, err := s.store.CompleteCharge(ctx, chargeID, result, price/2)
	if err != nil {
		return 500, models.Fail(err.Error())
	}
	if !done { // took longer than ChargeTimeout: the sweeper already returned the credits
		return 502, models.ChargeResult{ChargeID: chargeID, Status: "refunded", Error: "timed_out", Refunded: true,
			Available: s.store.Available(ctx, acc, r.AppID, now)}
	}
	return 201, models.ChargeResult{ChargeID: chargeID, Status: "completed", Charged: price, Result: result, Blocked: blocked, // a block is charged: the provider did the work
		Available: s.store.Available(ctx, acc, r.AppID, now)}
}

// debit locks the account, records the charge and takes credits from lots in one transaction.
// chargeID == "" means the request ends here (replay, conflict or 402).
func (s *Service) debit(ctx context.Context, acc string, r models.ChargeRequest, price int64, hash string) (string, int, models.ChargeResult) {
	now := s.clock.Now(ctx)
	var chargeID string
	var code int
	var res models.ChargeResult
	err := db.InTx(ctx, s.pool, func(q db.Q) error {
		if err := s.store.LockAccount(ctx, q, acc); err != nil {
			return err
		}
		// A copy with the same key waits on the account lock above, then finds the key and replays.
		id, inserted, err := s.store.InsertCharge(ctx, q, acc, r, price, hash, now)
		if err != nil {
			return err
		}
		if !inserted {
			code = -1 // replay after the transaction
			return errRollback
		}
		lots, err := s.store.EligibleLots(ctx, q, acc, r.AppID, now)
		if err != nil {
			return err
		}
		var avail int64
		for _, l := range lots {
			avail += l.Remaining
		}
		if avail < price {
			code, res = 402, models.ChargeResult{Status: "error", Error: "insufficient_credits", Available: avail, Required: price}
			return errRollback // the charge row goes too: a 402 is not stored
		}
		need := price
		for _, l := range lots {
			if need == 0 {
				break
			}
			take := min(need, l.Remaining)
			ok, err := s.store.Debit(ctx, q, l.ID, take)
			if err != nil {
				return err
			}
			if !ok {
				code, res = 409, models.Fail("lot_changed_retry")
				return errRollback
			}
			if err := s.store.Write(ctx, q, models.Entry{AccountID: acc, LotID: l.ID, ChargeID: id, Type: "charge",
				Delta: -take, Note: r.Action, At: now}); err != nil {
				return err
			}
			need -= take
		}
		chargeID = id
		return nil
	})
	switch {
	case code == -1:
		c, rr := s.replay(ctx, acc, r, hash)
		return "", c, rr
	case code != 0:
		return "", code, res
	case err != nil:
		return "", 500, models.Fail(err.Error())
	}
	return chargeID, 0, models.ChargeResult{}
}

// replay answers a repeated idempotency key from the stored charge.
func (s *Service) replay(ctx context.Context, acc string, r models.ChargeRequest, hash string) (int, models.ChargeResult) {
	id, status, stored, amount, result, err := s.store.StoredCharge(ctx, acc, r.Key)
	if err != nil {
		return 500, models.Fail(err.Error())
	}
	if stored != hash {
		return 422, models.Fail("idempotency_key_reused")
	}
	out := models.ChargeResult{ChargeID: id, Status: status, Charged: amount, Result: result,
		Available: s.store.Available(ctx, acc, r.AppID, s.clock.Now(ctx))}
	switch status {
	case "in_progress":
		out.Error = "in_progress"
		return 409, out
	case "refunded":
		out.Refunded, out.Charged = true, 0
		return 502, out
	}
	return 200, out
}

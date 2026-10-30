package app_test

import (
	"context"
	"sync"
	"testing"
)

// ---------- replay before checks, stale in_progress charges ----------

func TestIdempotency_ReplayAfterLicenseRevoked(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	if code, _ := e.charge("alice", "retouch", "ai.retouch", "k-rev", "x"); code != 201 {
		t.Fatalf("first call: %d", code)
	}
	subID := scalar[string](e, `SELECT s.id::text FROM subscriptions s JOIN offerings o ON o.id=s.offering_id WHERE o.code='off_design'`)
	if err := e.a.Billing.Refund(ctx, subID); err != nil {
		t.Fatal(err)
	}
	// The license is gone, but a retry of the same request must get the stored answer, not 403.
	if code, res := e.charge("alice", "retouch", "ai.retouch", "k-rev", "x"); code != 200 || res.Status != "completed" {
		t.Fatalf("replay after revoke: %d %+v, want 200 completed", code, res)
	}
	if code, _ := e.charge("alice", "retouch", "ai.retouch", "k-new", "x"); code != 403 {
		t.Fatalf("new key after revoke: %d, want 403", code)
	}
}

func TestSweeper_CrashAfterDebitReturnsCredits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	before := e.remaining("alice")
	e.a.Faults.CrashAI.Add(1)
	if code, res := e.charge("alice", "retouch", "ai.retouch", "k-crash", "x"); code != 500 || res.Status != "in_progress" {
		t.Fatalf("crash: %d %+v", code, res)
	}
	if spent := before - e.remaining("alice"); spent != 50 {
		t.Fatalf("after crash spent %d, want 50 (debited, not refunded)", spent)
	}
	if code, _ := e.charge("alice", "retouch", "ai.retouch", "k-crash", "x"); code != 409 {
		t.Fatalf("retry while in_progress: %d, want 409", code)
	}
	if n, err := e.a.Credits.SweepStale(ctx); err != nil || n != 0 {
		t.Fatalf("sweep before timeout: n=%d err=%v, want 0 (a live request must not be swept)", n, err)
	}
	if err := e.a.Clock.Advance(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// Every instance sweeps at once: credits come back exactly once.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = e.a.Credits.SweepStale(ctx) }()
	}
	wg.Wait()
	if got := e.remaining("alice"); got != before {
		t.Fatalf("after sweep remaining %d, want %d", got, before)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM credit_ledger e JOIN credit_charges c ON c.id=e.charge_id WHERE c.idempotency_key='k-crash' AND e.type='refund'`); n != 1 {
		t.Fatalf("refund ledger rows = %d, want 1", n)
	}
	// A late refund attempt (the original request waking up, a second sweeper) must not return the credits again.
	id := scalar[string](e, `SELECT id::text FROM credit_charges WHERE idempotency_key='k-crash'`)
	if err := e.a.Credits.RefundCharge(ctx, id, "late duplicate"); err != nil {
		t.Fatal(err)
	}
	if got := e.remaining("alice"); got != before {
		t.Fatalf("second refund changed remaining to %d, want %d", got, before)
	}
	if code, res := e.charge("alice", "retouch", "ai.retouch", "k-crash", "x"); code != 502 || !res.Refunded {
		t.Fatalf("retry after sweep: %d %+v, want 502 refunded", code, res)
	}
	e.assertLedger()
}

func TestSweeper_RefundIntoExpiredLotIsBurned(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	e.a.Faults.CrashAI.Add(1)
	e.charge("alice", "retouch", "ai.retouch", "k-exp", "x")
	if err := e.a.Billing.Advance(ctx, 31); err != nil { // the Design+ lot of period 1 expires
		t.Fatal(err)
	}
	if _, err := e.a.Credits.SweepStale(ctx); err != nil {
		t.Fatal(err)
	}
	// Period 1's lot is the older of the two Design+ lots; after the sweep it must hold nothing.
	if left := scalar[int64](e, `SELECT remaining FROM credit_lots WHERE label LIKE 'Design+%' ORDER BY expires_at LIMIT 1`); left != 0 {
		t.Fatalf("expired lot holds %d credits after the sweep, want 0", left)
	}
	e.assertLedger()
}

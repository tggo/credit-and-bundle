package app_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// ---------- races ----------

func TestRace_OneBalanceManyDevices(t *testing.T) {
	e := newEnv(t)
	e.buy("bob", "off_appshop")
	e.buy("bob", "off_pack500")
	// 500 credits, 20 devices × 80 → exactly 6 succeed, 14 get 402, nothing else.
	got := countCodes(e.chargeN(20, "bob", "appshop", "ai.generate", func(i int) string { return fmt.Sprintf("dev-%d", i) }))
	if got[201] != 6 || got[402] != 14 || len(got) != 2 {
		t.Fatalf("want 6×201 + 14×402, got %v", got)
	}
	if r := e.remaining("bob"); r != 20 {
		t.Fatalf("remaining = %d, want 20", r)
	}
	e.assertLedger()
}

func TestRace_ManyCustomersNoDeadlock(t *testing.T) {
	e := newEnv(t)
	for _, c := range []string{"alice", "bob"} {
		e.buy(c, "off_design")
		e.buy(c, "off_pack500")
	}
	targets := []struct{ cust, app, action string }{
		{"alice", "appshop", "ai.generate"}, {"alice", "retouch", "ai.retouch"},
		{"bob", "appshop", "ai.generate"}, {"bob", "notes", "ai.summarize"},
	}
	results := make([][]int, len(targets))
	var wg sync.WaitGroup
	for i, tg := range targets {
		wg.Add(1)
		go func(i int, cust, app, action string) {
			defer wg.Done()
			results[i] = e.chargeN(10, cust, app, action, func(j int) string { return fmt.Sprintf("%s-%s-%d", cust, app, j) })
		}(i, tg.cust, tg.app, tg.action)
	}
	wg.Wait()
	for i, r := range results {
		if got := countCodes(r); got[201] != 10 {
			t.Fatalf("%v: want 10×201 (no deadlock, no 500), got %v", targets[i], got)
		}
	}
	// alice: 10×80 + 10×50 = 1300 from the bundle lot; the pack is untouched.
	if r := e.remaining("alice"); r != 10000+500-1300 {
		t.Fatalf("alice remaining = %d", r)
	}
	e.assertLedger()
}

func TestRace_ChargeSpanningTwoLots(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	e.buy("alice", "off_pack500")
	// Drain the bundle lot down to 30 so the next 80-credit charge takes 30 from it and 50 from the pack.
	if _, err := e.a.Pool.Exec(ctx, `WITH l AS (SELECT id, account_id, remaining FROM credit_lots WHERE scope='bundle')
		INSERT INTO credit_ledger (account_id, lot_id, type, delta, note, created_at) SELECT account_id, id, 'charge', 30 - remaining, 'test drain', now() FROM l`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.Pool.Exec(ctx, `UPDATE credit_lots SET remaining = 30 WHERE scope='bundle'`); err != nil {
		t.Fatal(err)
	}
	if code, res := e.charge("alice", "appshop", "ai.generate", "span-1", "x"); code != 201 {
		t.Fatalf("charge: %d %+v", code, res)
	}
	if b := scalar[int64](e, `SELECT remaining FROM credit_lots WHERE scope='bundle'`); b != 0 {
		t.Fatalf("bundle lot remaining = %d, want 0", b)
	}
	if p := scalar[int64](e, `SELECT remaining FROM credit_lots WHERE scope='platform'`); p != 450 {
		t.Fatalf("pack remaining = %d, want 450", p)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM credit_ledger x JOIN credit_charges c ON c.id=x.charge_id WHERE c.idempotency_key='span-1'`); n != 2 {
		t.Fatalf("ledger entries for the charge = %d, want 2", n)
	}
	e.assertLedger()
}

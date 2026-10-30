package app_test

import (
	"context"
	"fmt"
	"math"
	"testing"
)

// ---------- data flows ----------

func TestFlow_ScopeAndSpendOrder(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	e.buy("alice", "off_pack500")
	if code, _ := e.charge("alice", "appshop", "ai.generate", "a1", "x"); code != 201 {
		t.Fatalf("appshop: %d", code)
	}
	if p := scalar[int64](e, `SELECT remaining FROM credit_lots WHERE scope='platform'`); p != 500 {
		t.Fatalf("pack touched: %d", p)
	}
	if code, _ := e.charge("alice", "cleanbox", "ai.scan", "c1", "x"); code != 403 {
		t.Fatalf("cleanbox without license: %d, want 403", code)
	}
	e.buy("alice", "off_cmm")
	b, _ := e.a.Credits.Balance(ctx, e.cust("alice"), e.app("cleanbox"))
	if b.Available != 500 || b.Total != 10000-80+500 {
		t.Fatalf("cleanbox balance = %+v", b)
	}
	if code, _ := e.charge("alice", "cleanbox", "ai.scan", "c2", "x"); code != 201 {
		t.Fatalf("cleanbox: %d", code)
	}
	if p := scalar[int64](e, `SELECT remaining FROM credit_lots WHERE scope='platform'`); p != 470 {
		t.Fatalf("pack remaining = %d, want 470", p)
	}
	e.assertLedger()
}

func TestFlow_ProviderFailureRefundsSameLots(t *testing.T) {
	e := newEnv(t)
	e.buy("alice", "off_design")
	before := e.remaining("alice")
	e.a.Faults.FailAI.Store(1)
	if code, res := e.charge("alice", "notes", "ai.summarize", "f1", "x"); code != 502 || !res.Refunded {
		t.Fatalf("want 502 refunded, got %d %+v", code, res)
	}
	if e.remaining("alice") != before {
		t.Fatalf("credits not returned")
	}
	if n := scalar[int64](e, `SELECT count(*) FROM credit_ledger WHERE type='refund'`); n != 1 {
		t.Fatalf("refund entries = %d", n)
	}
	e.a.Faults.BlockAI.Store(1)
	if code, _ := e.charge("alice", "notes", "ai.summarize", "b1", "x"); code != 201 {
		t.Fatalf("blocked: %d", code)
	}
	if before-e.remaining("alice") != 20 {
		t.Fatalf("content block should be charged")
	}
	e.assertLedger()
}

func TestFlow_PartialRefund(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	for i := 0; i < 5; i++ {
		e.charge("alice", "appshop", "ai.generate", fmt.Sprintf("r%d", i), "x") // 400 credits
	}
	if err := e.a.Billing.Advance(ctx, 15); err != nil {
		t.Fatal(err)
	}
	subID := scalar[string](e, `SELECT s.id::text FROM subscriptions s JOIN offerings o ON o.id=s.offering_id WHERE o.code='off_design'`)
	q, err := e.a.Billing.Quote(ctx, subID)
	if err != nil {
		t.Fatal(err)
	}
	// $30 → apps $24, credits $6. Half the period left, 9600/10000 unspent.
	want := int64(math.Round(2400*0.5 + 600*0.96))
	if q.AppsPart != 2400 || q.CredPart != 600 || math.Abs(q.UnusedFrac-0.5) > 0.01 || q.Refund != want {
		t.Fatalf("quote %+v, want refund %d", q, want)
	}
	if err := e.a.Billing.Refund(ctx, subID); err != nil {
		t.Fatal(err)
	}
	if st := scalar[string](e, `SELECT status FROM subscriptions WHERE id=$1`, subID); st != "canceled" {
		t.Fatalf("subscription %s", st)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM licenses WHERE subscription_id=$1 AND status='active'`, subID); n != 0 {
		t.Fatalf("active licenses left: %d", n)
	}
	if r := e.remaining("alice"); r != 0 {
		t.Fatalf("lots not revoked: %d", r)
	}
	if n := scalar[int64](e, `SELECT -SUM(delta) FROM credit_ledger WHERE type='revoke'`); n != 9600 {
		t.Fatalf("revoked %d, want 9600", n)
	}
	e.assertLedger()
}

func TestFlow_RenewalAndExpiry(t *testing.T) {
	e := newEnv(t)
	e.buy("alice", "off_design")
	e.charge("alice", "appshop", "ai.generate", "e1", "x")
	if err := e.a.Billing.Advance(context.Background(), 31); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM payments`); n != 2 {
		t.Fatalf("payments = %d, want 2 (renewal)", n)
	}
	if n := scalar[int64](e, `SELECT -SUM(delta) FROM credit_ledger WHERE type='expire'`); n != 9920 {
		t.Fatalf("expired %d, want 9920", n)
	}
	if r := e.remaining("alice"); r != 10000 {
		t.Fatalf("remaining %d, want 10000 (fresh lot)", r)
	}
	e.assertLedger()
}

func TestFlow_SubscriptionWithAndWithoutCredits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("bob", "off_cmm") // plain subscription: license only
	if n := scalar[int64](e, `SELECT count(*) FROM credit_lots`); n != 0 {
		t.Fatalf("plain subscription granted %d lot(s)", n)
	}
	if code, _ := e.charge("bob", "cleanbox", "ai.scan", "p1", "x"); code != 402 {
		t.Fatalf("plain subscription AI: %d, want 402", code)
	}
	e.buy("alice", "off_cmm_ai") // 300 credits usable only in CleanBox
	if sc := scalar[string](e, `SELECT scope || '/' || source_type FROM credit_lots`); sc != "bundle/subscription" {
		t.Fatalf("lot scope/source = %s", sc)
	}
	if code, _ := e.charge("alice", "cleanbox", "ai.scan", "s1", "x"); code != 201 {
		t.Fatalf("cleanbox AI: %d", code)
	}
	e.buy("alice", "off_appshop")
	if b, _ := e.a.Credits.Balance(ctx, e.cust("alice"), e.app("appshop")); b.Available != 0 || b.Total != 270 {
		t.Fatalf("CleanBox credits leaked into App Shop: %+v", b)
	}
	if err := e.a.Billing.Advance(ctx, 31); err != nil {
		t.Fatal(err)
	}
	if b, _ := e.a.Credits.Balance(ctx, e.cust("alice"), e.app("cleanbox")); b.Available != 300 {
		t.Fatalf("after renewal available = %d, want 300", b.Available)
	}
	e.assertLedger()
}

func TestFlow_InAppPackOnlyForItsApp(t *testing.T) {
	e := newEnv(t)
	e.buy("bob", "off_design")        // licenses App Shop, Face Retouch, Notes AI + 10 000 bundle credits
	e.buy("bob", "off_appshop_cr300") // bought inside App Shop
	if n := scalar[int64](e, `SELECT count(*) FROM lot_scope_apps a JOIN credit_lots l ON l.id=a.lot_id
		JOIN applications ap ON ap.id=a.application_id WHERE l.label LIKE 'App Shop credits 300%' AND ap.code='appshop'`); n != 1 {
		t.Fatalf("in-app pack lot scoped to App Shop: %d rows, want 1", n)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM lot_scope_apps a JOIN credit_lots l ON l.id=a.lot_id WHERE l.label LIKE 'App Shop credits 300%'`); n != 1 {
		t.Fatalf("in-app pack scoped to %d apps, want exactly 1", n)
	}
	// Spend the bundle down so only the pack could pay; Face Retouch must not see it.
	if _, err := e.a.Pool.Exec(context.Background(), `UPDATE credit_lots SET remaining=0 WHERE label LIKE 'Design+%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.Pool.Exec(context.Background(), `INSERT INTO credit_ledger (account_id, lot_id, type, delta, note, created_at)
		SELECT account_id, id, 'expire', -granted, 'test drain', now() FROM credit_lots WHERE label LIKE 'Design+%'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.charge("bob", "retouch", "ai.retouch", "k1", "x"); code != 402 {
		t.Fatalf("Face Retouch with only an App Shop pack: %d, want 402", code)
	}
	if code, _ := e.charge("bob", "appshop", "ai.generate", "k2", "x"); code != 201 {
		t.Fatalf("App Shop from its own pack: %d, want 201", code)
	}
	if got := scalar[int64](e, `SELECT remaining FROM credit_lots WHERE label LIKE 'App Shop credits 300%'`); got != 220 {
		t.Fatalf("pack remaining %d, want 220", got)
	}
	e.assertLedger()
}

// A refund of the renewal paid after past_due takes back only what that payment bought:
// the previous period's lot and access stay until that period ends.
func TestFlow_RefundAfterPastDueKeepsEarlierPeriod(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_cmm_ai") // 300 credits for CleanBox, period 1
	if code, _ := e.charge("alice", "cleanbox", "ai.scan", "p1", "x"); code != 201 {
		t.Fatalf("charge: %d", code)
	}
	if err := e.a.Billing.PastDue(ctx, e.cust("alice")); err != nil { // renewal declined, then paid: period 2
		t.Fatal(err)
	}
	if got := e.remaining("alice"); got != 270+300 {
		t.Fatalf("before refund %d, want 570 (270 left of period 1 + 300 of period 2)", got)
	}
	subID := scalar[string](e, `SELECT s.id::text FROM subscriptions s JOIN offerings o ON o.id=s.offering_id WHERE o.code='off_cmm_ai'`)
	if err := e.a.Billing.Refund(ctx, subID); err != nil { // refunds the latest payment (period 2)
		t.Fatal(err)
	}
	if got := e.remaining("alice"); got != 270 {
		t.Fatalf("after refund %d, want 270: only period 2's lot is revoked", got)
	}
	if code, _ := e.charge("alice", "cleanbox", "ai.scan", "p2", "x"); code != 201 {
		t.Fatalf("period 1 is still paid, license must still work: %d", code)
	}
	if err := e.a.Billing.Advance(ctx, 31); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.charge("alice", "cleanbox", "ai.scan", "p3", "x"); code != 403 {
		t.Fatalf("after period 1 ends: %d, want 403", code)
	}
	e.assertLedger()
}

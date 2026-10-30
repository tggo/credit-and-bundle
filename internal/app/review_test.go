package app_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	billmodels "bundle-credit-test/internal/billing/models"
	credmodels "bundle-credit-test/internal/credits/models"
)

// ---------- refunds against dead lots, refunds racing charges, early payment.refunded ----------

func TestRefund_IntoRevokedLotIsBurned(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design")
	e.a.Faults.CrashAI.Add(1)
	if code, _ := e.charge("alice", "retouch", "ai.retouch", "k-rv", "x"); code != 500 {
		t.Fatalf("crash: %d", code)
	}
	subID := scalar[string](e, `SELECT s.id::text FROM subscriptions s JOIN offerings o ON o.id=s.offering_id WHERE o.code='off_design'`)
	if err := e.a.Billing.Refund(ctx, subID); err != nil { // the payment's lots are zeroed while the charge hangs
		t.Fatal(err)
	}
	id := scalar[string](e, `SELECT id::text FROM credit_charges WHERE idempotency_key='k-rv'`)
	if err := e.a.Credits.RefundCharge(ctx, id, "provider 5xx"); err != nil {
		t.Fatal(err)
	}
	if left := e.remaining("alice"); left != 0 {
		t.Fatalf("refund put %d credits into a revoked lot, want 0", left)
	}
	e.assertLedger()
}

// A charge took the lot to exactly 0, so the payment refund had nothing to zero; the credits it returns later must still burn.
func TestRefund_IntoDrainedRevokedLotIsBurned(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_design") // one 10k lot
	left := e.remaining("alice")
	if _, err := e.a.Pool.Exec(ctx, `INSERT INTO action_prices (application_id, action_code, credits, valid_from)
		SELECT id, 'ai.retouch', $1, '2026-02-01' FROM applications WHERE code='retouch'`, left); err != nil {
		t.Fatal(err)
	}
	e.a.Faults.CrashAI.Add(1)
	if code, _ := e.charge("alice", "retouch", "ai.retouch", "k-drain", "x"); code != 500 {
		t.Fatalf("crash: %d", code)
	}
	if r := e.remaining("alice"); r != 0 {
		t.Fatalf("charge left %d, want the lot drained to 0", r)
	}
	subID := scalar[string](e, `SELECT id::text FROM subscriptions`)
	if err := e.a.Billing.Refund(ctx, subID); err != nil {
		t.Fatal(err)
	}
	id := scalar[string](e, `SELECT id::text FROM credit_charges WHERE idempotency_key='k-drain'`)
	if err := e.a.Credits.RefundCharge(ctx, id, "provider 5xx"); err != nil {
		t.Fatal(err)
	}
	if r := e.remaining("alice"); r != 0 {
		t.Fatalf("refund put %d credits into a drained revoked lot, want 0", r)
	}
	e.assertLedger()
}

func TestRefund_RacesChargesWithoutDeadlock(t *testing.T) {
	e := newEnv(t)
	e.buy("alice", "off_design")
	e.a.Faults.FailAI.Store(100) // half of the calls fail and refund while the others debit the same lot
	cust, appID := e.cust("alice"), e.app("retouch")
	var wg sync.WaitGroup
	codes := make([]int, 200)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Duration(i) * 2 * time.Millisecond) // spread: debits keep coming while earlier calls refund
			codes[i], _ = e.a.Credits.Charge(context.Background(), credmodels.ChargeRequest{CustomerID: cust, AppID: appID,
				Action: "ai.retouch", Key: "k-race-" + strconv.Itoa(i), Input: "x"})
		}(i)
	}
	wg.Wait()
	t.Logf("codes: %v", countCodes(codes))
	if n := countCodes(codes)[500]; n != 0 {
		t.Fatalf("%d charges failed with 500 (deadlock?): %v", n, codes)
	}
	e.assertLedger()
}

func TestWebhooks_RefundBeforePaidIsParked(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.a.Faults.LoseWebhook.Store(1)
	e.buy("alice", "off_pack500")
	rows, _, err := e.a.Billing.Reconcile(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("reconcile: %+v %v", rows, err)
	}
	ref := rows[0].PaymentRef
	subRef := scalar[string](e, `SELECT provider_sub_ref FROM subscriptions`)
	e.a.Billing.Deliver(ctx, billmodels.Event{EventID: "evt_early_refund", Type: "payment.refunded", SubRef: subRef, PaymentRef: ref})
	if st := scalar[string](e, `SELECT status FROM webhook_events WHERE event_id='evt_early_refund'`); st != "parked" {
		t.Fatalf("early refund: %s, want parked", st)
	}
	if err := e.a.Billing.FixRecon(ctx, ref); err != nil { // invoice.paid arrives; the parked refund follows it
		t.Fatal(err)
	}
	if st := scalar[string](e, `SELECT status FROM webhook_events WHERE event_id='evt_early_refund'`); st != "applied" {
		t.Fatalf("parked refund after paid: %s, want applied", st)
	}
	if left := e.remaining("alice"); left != 0 {
		t.Fatalf("credits of a refunded payment: %d, want 0", left)
	}
	e.assertLedger()
}

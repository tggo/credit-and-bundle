package app_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	billmodels "bundle-credit-test/internal/billing/models"
)

// ---------- webhooks (simulated Billing) ----------

func TestWebhooks_OutOfOrderAndDuplicates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.a.Billing.Purchase(ctx, e.cust("bob"), "bob", e.offering("off_design"), true); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM payments`); n != 1 {
		t.Fatalf("parked invoice.paid not applied: payments = %d", n)
	}
	ev := billmodels.Event{EventID: "evt_" + scalar[string](e, `SELECT payment_ref FROM payments`) + "_paid", Type: "invoice.paid",
		SubRef: scalar[string](e, `SELECT provider_sub_ref FROM subscriptions`), PaymentRef: scalar[string](e, `SELECT payment_ref FROM payments`),
		CustomerID: e.cust("bob"), OfferingID: e.offering("off_design"), AmountMinor: 3000}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			x := ev
			if i%2 == 1 {
				x.EventID = fmt.Sprintf("evt_resent_%d", i)
			}
			e.a.Billing.Handle(ctx, x)
		}(i)
	}
	wg.Wait()
	for q, want := range map[string]int64{`SELECT count(*) FROM payments`: 1, `SELECT count(*) FROM credit_lots`: 1, `SELECT count(*) FROM licenses`: 3} {
		if n := scalar[int64](e, q); n != want {
			t.Fatalf("%s = %d, want %d", q, n, want)
		}
	}
	e.assertLedger()
}

func TestWebhooks_TerminalStateAndPastDue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.buy("alice", "off_cmm")
	if err := e.a.Billing.PastDue(ctx, e.cust("alice")); err != nil {
		t.Fatal(err)
	}
	if st := scalar[string](e, `SELECT status FROM subscriptions`); st != "active" {
		t.Fatalf("after failed → paid: %s, want active", st)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM webhook_events WHERE type='invoice.failed' AND status='applied'`); n != 1 {
		t.Fatalf("invoice.failed not applied")
	}
	if err := e.a.Billing.LateCreated(ctx, e.cust("alice")); err != nil {
		t.Fatal(err)
	}
	if st := scalar[string](e, `SELECT status FROM subscriptions`); st != "canceled" {
		t.Fatalf("late created revived the subscription: %s", st)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM webhook_events WHERE type='subscription.created' AND status='rejected'`); n != 1 {
		t.Fatalf("late subscription.created not rejected")
	}
}

func TestReconcile_LostWebhookFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.a.Faults.LoseWebhook.Store(1)
	e.buy("alice", "off_pack500")
	if r := e.remaining("alice"); r != 0 {
		t.Fatalf("credits granted despite lost webhook")
	}
	rows, _, err := e.a.Billing.Reconcile(ctx)
	if err != nil || len(rows) != 1 || rows[0].Kind != "missing_ours" {
		t.Fatalf("reconcile: %+v %v", rows, err)
	}
	if err := e.a.Billing.FixRecon(ctx, rows[0].PaymentRef); err != nil {
		t.Fatal(err)
	}
	if r := e.remaining("alice"); r != 500 {
		t.Fatalf("after replay remaining = %d", r)
	}
	if rows, _, _ := e.a.Billing.Reconcile(ctx); len(rows) != 0 {
		t.Fatalf("still discrepancies: %+v", rows)
	}
}

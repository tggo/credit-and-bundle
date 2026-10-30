package app_test

import (
	"testing"
)

// ---------- idempotency ----------

func TestIdempotency_SameKeyConcurrent(t *testing.T) {
	e := newEnv(t)
	e.buy("alice", "off_design")
	before := e.remaining("alice")
	got := countCodes(e.chargeN(10, "alice", "retouch", "ai.retouch", func(int) string { return "same-key" }))
	if got[201] != 1 || got[201]+got[409]+got[200] != 10 {
		t.Fatalf("want exactly one 201, rest 409/200, got %v", got)
	}
	if n := scalar[int64](e, `SELECT count(*) FROM credit_charges WHERE idempotency_key='same-key'`); n != 1 {
		t.Fatalf("charges = %d, want 1", n)
	}
	if spent := before - e.remaining("alice"); spent != 50 {
		t.Fatalf("spent %d, want 50 (charged once)", spent)
	}
	if code, res := e.charge("alice", "retouch", "ai.retouch", "same-key", "same input"); code != 200 || res.Status != "completed" || len(res.Result) == 0 {
		t.Fatalf("replay: %d %+v", code, res)
	}
	if code, _ := e.charge("alice", "retouch", "ai.retouch", "same-key", "other input"); code != 422 {
		t.Fatalf("key reuse with other body: %d, want 422", code)
	}
	e.assertLedger()
}

func TestIdempotency_402IsNotStored(t *testing.T) {
	e := newEnv(t)
	e.buy("bob", "off_appshop") // license, no credits
	if code, _ := e.charge("bob", "appshop", "ai.generate", "k402", "x"); code != 402 {
		t.Fatalf("want 402, got %d", code)
	}
	e.buy("bob", "off_pack500")
	if code, _ := e.charge("bob", "appshop", "ai.generate", "k402", "x"); code != 201 {
		t.Fatalf("retry after top-up: want 201, got %d", code)
	}
}

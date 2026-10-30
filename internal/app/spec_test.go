package app_test

// Specifications: Given (steps that already happened) → When (one step) → Then (what that step produced).
// Then is compared field by field (see spec_diff_test.go); charge ids are symbolic — charge#1 is the first
// charge of the spec — and lots are named by their label, so a spec reads as text and survives new uuids.
// Only observable results are checked (status, new ledger rows, lot balances), never internal state.
//
//	go test ./internal/app -run TestSpecs -specdoc=$PWD/SPECS.md -specreport=$PWD/specs.html
//
// -specreport adds the SQL each When ran and its plan with seq scans disabled (spec_sql_test.go): a Seq Scan
// still in the plan means no index can serve that query.

import (
	"context"
	"fmt"
	"testing"

	credmodels "bundle-credit-test/internal/credits/models"
)

type spec struct {
	Name  string
	Given []step
	When  step
	Then  then
}

// then is what the When step produced. Zero Status is not checked; Lots lists only the lots to check.
type then struct {
	Status int
	Ledger []entry
	Lots   map[string]int64
}

type entry struct {
	Type, Lot, Charge string
	Delta             int64
}

// run holds what a spec run has seen so far: the last status and charge ids in the order they appeared.
type run struct {
	e       *env
	status  int
	charges map[string]string // real id → charge#N
}

func (r *run) sym(id string) string {
	if id == "" {
		return ""
	}
	if s, ok := r.charges[id]; ok {
		return s
	}
	s := fmt.Sprintf("charge#%d", len(r.charges)+1)
	r.charges[id] = s
	return s
}

type step interface {
	do(r *run)
	String() string
}

type buy struct{ Who, Offering string }

func (s buy) do(r *run)      { r.e.buy(s.Who, s.Offering); r.status = 0 }
func (s buy) String() string { return fmt.Sprintf("%s buys %s", s.Who, s.Offering) }

type charge struct{ Who, App, Action, Key string }

func (s charge) do(r *run) {
	var res credmodels.ChargeResult
	r.status, res = r.e.charge(s.Who, s.App, s.Action, s.Key, "spec input")
	r.sym(res.ChargeID)
}
func (s charge) String() string {
	return fmt.Sprintf("%s runs %s in %s (key %s)", s.Who, s.Action, s.App, s.Key)
}

type days struct{ N int }

func (s days) do(r *run) {
	if err := r.e.a.Billing.Advance(context.Background(), s.N); err != nil {
		r.e.t.Fatalf("advance %d days: %v", s.N, err)
	}
	r.status = 0
}
func (s days) String() string { return fmt.Sprintf("%d days pass", s.N) }

type aiFails struct{}

func (aiFails) do(r *run)      { r.e.a.Faults.FailAI.Store(1) }
func (aiFails) String() string { return "the AI provider returns 5xx" }

type aiBlocks struct{}

func (aiBlocks) do(r *run)      { r.e.a.Faults.BlockAI.Store(1) }
func (aiBlocks) String() string { return "the AI provider refuses the content" }

// result is one checked spec: its differences from Then and the queries its When step ran.
type result struct {
	Spec    spec
	Issues  []issue
	Queries []query
}

// check runs the spec on a fresh database, recording the SQL of the When step only.
func (sp spec) check(t *testing.T) result {
	tr := &tracer{}
	r := &run{e: newEnvWith(t, tr), charges: map[string]string{}}
	for _, g := range sp.Given {
		g.do(r)
	}
	mark := scalar[int64](r.e, `SELECT COALESCE(MAX(id),0) FROM credit_ledger`)
	tr.on.Store(true)
	sp.When.do(r)
	tr.on.Store(false)
	r.e.assertLedger()
	return result{Spec: sp, Issues: diff("", sp.Then, r.observe(mark, sp.Then)), Queries: tr.explain(r.e)}
}

// lotNames names lots by their label without the period date ("Design+ bundle · Oct 28" → "Design+ bundle"),
// in creation order; a second lot with the same name becomes "Design+ bundle (2)".
func (r *run) lotNames() (names map[string]string, left map[string]int64) {
	rows, err := r.e.a.Pool.Query(context.Background(),
		`SELECT id::text, split_part(label, ' · ', 1), remaining FROM credit_lots ORDER BY created_at, id`)
	if err != nil {
		r.e.t.Fatalf("lots: %v", err)
	}
	defer rows.Close()
	names, left, seen := map[string]string{}, map[string]int64{}, map[string]int{}
	for rows.Next() {
		var id, name string
		var rem int64
		if err := rows.Scan(&id, &name, &rem); err != nil {
			r.e.t.Fatalf("lot row: %v", err)
		}
		if seen[name]++; seen[name] > 1 {
			name = fmt.Sprintf("%s (%d)", name, seen[name])
		}
		names[id], left[name] = name, rem
	}
	return names, left
}

// observe reads what When produced, in the shape of the expectation.
func (r *run) observe(mark int64, want then) then {
	got := then{Lots: map[string]int64{}}
	if want.Status != 0 {
		got.Status = r.status
	}
	names, left := r.lotNames()
	rows, err := r.e.a.Pool.Query(context.Background(), `SELECT type, lot_id::text, COALESCE(charge_id::text,''), delta
		FROM credit_ledger WHERE id > $1 ORDER BY id`, mark)
	if err != nil {
		r.e.t.Fatalf("ledger: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var x entry
		if err := rows.Scan(&x.Type, &x.Lot, &x.Charge, &x.Delta); err != nil {
			r.e.t.Fatalf("ledger row: %v", err)
		}
		x.Lot, x.Charge = names[x.Lot], r.sym(x.Charge)
		got.Ledger = append(got.Ledger, x)
	}
	for name := range want.Lots {
		if v, ok := left[name]; ok {
			got.Lots[name] = v
		}
	}
	return got
}

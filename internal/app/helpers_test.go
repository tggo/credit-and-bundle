package app_test

// Integration tests against a real Postgres (docker compose up -d), through the wired app.
// Each test recreates the schema in the test database.

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bundle-credit-test/internal/app"
	credmodels "bundle-credit-test/internal/credits/models"
)

type env struct {
	t *testing.T
	a *app.App
}

func newEnv(t *testing.T) *env { return newEnvWith(t, nil) }

// newEnvWith lets a test watch every query the app runs (specs record them for the query-plan report).
func newEnvWith(t *testing.T, tracer pgx.QueryTracer) *env {
	t.Helper()
	dsn := "postgres://bc:bc@localhost:55433/bundlecredit_test?sslmode=disable&timezone=UTC&pool_max_conns=40"
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("db config: %v", err)
	}
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("test database not reachable (%v); run: docker compose up -d", err)
	}
	t.Cleanup(pool.Close)
	a := app.New(pool, "")
	if err := a.Start(ctx, false); err != nil {
		t.Fatalf("start: %v", err)
	}
	return &env{t: t, a: a}
}

func scalar[T any](e *env, q string, args ...any) T {
	e.t.Helper()
	var v T
	if err := e.a.Pool.QueryRow(context.Background(), q, args...).Scan(&v); err != nil {
		e.t.Fatalf("query %q: %v", q, err)
	}
	return v
}

func (e *env) cust(handle string) string {
	return scalar[string](e, `SELECT id::text FROM customers WHERE handle=$1`, handle)
}
func (e *env) app(code string) string {
	return scalar[string](e, `SELECT id::text FROM applications WHERE code=$1`, code)
}
func (e *env) offering(code string) string {
	return scalar[string](e, `SELECT id::text FROM offerings WHERE code=$1`, code)
}

func (e *env) buy(handle, offering string) {
	e.t.Helper()
	if err := e.a.Billing.Purchase(context.Background(), e.cust(handle), handle, e.offering(offering), false); err != nil {
		e.t.Fatalf("purchase %s: %v", offering, err)
	}
}

func (e *env) charge(handle, app, action, key, input string) (int, credmodels.ChargeResult) {
	return e.a.Credits.Charge(context.Background(), credmodels.ChargeRequest{CustomerID: e.cust(handle), AppID: e.app(app),
		Action: action, Key: key, Input: input})
}

func (e *env) remaining(handle string) int64 {
	return scalar[int64](e, `SELECT COALESCE(SUM(l.remaining),0) FROM credit_lots l JOIN credit_accounts a ON a.id=l.account_id
		JOIN customers c ON c.id=a.customer_id WHERE c.handle=$1`, handle)
}

// assertLedger checks the core invariant: every lot's remaining equals the sum of its ledger entries, and is never negative.
func (e *env) assertLedger() {
	e.t.Helper()
	bad := scalar[int64](e, `SELECT count(*) FROM credit_lots l
		WHERE l.remaining <> COALESCE((SELECT SUM(delta) FROM credit_ledger x WHERE x.lot_id = l.id), 0) OR l.remaining < 0`)
	if bad != 0 {
		e.t.Fatalf("ledger invariant broken on %d lot(s)", bad)
	}
}

func countCodes(codes []int) map[int]int {
	m := map[int]int{}
	for _, c := range codes {
		m[c]++
	}
	return m
}

func (e *env) chargeN(n int, handle, app, action string, key func(i int) string) []int {
	codes := make([]int, n)
	cust, appID := e.cust(handle), e.app(app)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.a.Credits.Charge(context.Background(), credmodels.ChargeRequest{CustomerID: cust, AppID: appID,
				Action: action, Key: key(i), Input: "same input"})
		}(i)
	}
	wg.Wait()
	return codes
}

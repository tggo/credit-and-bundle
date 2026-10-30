// Package app is the composition root: it builds every storage and service once and wires them together.
// cmd/server and the integration tests both start from here.
package app

import (
	"context"
	"time"

	billsvc "bundle-credit-test/internal/billing/services"
	billstore "bundle-credit-test/internal/billing/storages"
	boardstore "bundle-credit-test/internal/board/storages"
	catstore "bundle-credit-test/internal/catalog/storages"
	credsvc "bundle-credit-test/internal/credits/services"
	credstore "bundle-credit-test/internal/credits/storages"
	custstore "bundle-credit-test/internal/customer/storages"
	licstore "bundle-credit-test/internal/license/storages"
	"bundle-credit-test/internal/platform/aimock"
	"bundle-credit-test/internal/platform/clock"
	"bundle-credit-test/internal/platform/db"
	"bundle-credit-test/internal/platform/faults"
	"bundle-credit-test/internal/platform/plantuml"
	"bundle-credit-test/internal/platform/schema"
	"bundle-credit-test/internal/platform/sse"
)

type App struct {
	Pool      *db.Pool
	Hub       *sse.Hub
	Clock     *clock.Clock
	Faults    *faults.Faults
	Customers *custstore.Store
	Catalog   *catstore.Store
	Licenses  *licstore.Store
	Credits   *credsvc.Service
	Billing   *billsvc.Service
	Boards    *boardstore.Store
	PlantUML  *plantuml.Client
}

func New(pool *db.Pool, plantumlURL string) *App {
	a := &App{Pool: pool, PlantUML: plantuml.New(plantumlURL), Hub: sse.NewHub(), Clock: clock.New(pool), Faults: &faults.Faults{},
		Customers: custstore.New(pool), Catalog: catstore.New(pool), Licenses: licstore.New(pool), Boards: boardstore.New(pool)}
	ai := &aimock.Provider{Faults: a.Faults, Delay: 250 * time.Millisecond}
	a.Credits = credsvc.New(pool, credstore.New(pool), a.Licenses, a.Catalog, ai, a.Clock, a.Hub)
	a.Billing = billsvc.New(pool, billstore.New(pool), a.Catalog, a.Licenses, a.Credits, a.Clock, a.Hub, a.Faults)
	return a
}

// Reset recreates the simulator data; boards are kept.
func (a *App) Reset(ctx context.Context) error {
	if err := schema.Reset(ctx, a.Pool); err != nil {
		return err
	}
	if err := schema.Describe(ctx, a.Pool); err != nil {
		return err
	}
	a.Faults.Reset()
	a.Hub.ClearLog()
	return nil
}

// Start prepares the database: boards always, simulator data on an empty database or when keep is false.
func (a *App) Start(ctx context.Context, keep bool) error {
	if err := schema.EnsureBoards(ctx, a.Pool); err != nil {
		return err
	}
	if !keep || !schema.Seeded(ctx, a.Pool) {
		if err := a.Reset(ctx); err != nil {
			return err
		}
	}
	return schema.Describe(ctx, a.Pool)
}

// RunSweeper refunds charges stuck in_progress (see credits.ChargeTimeout) every interval until ctx ends.
// Every instance runs it; the conditional status update makes concurrent sweeps safe.
func (a *App) RunSweeper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Sweep(ctx)
		}
	}
}

// Sweep runs one pass and reports it in the live log.
func (a *App) Sweep(ctx context.Context) {
	n, err := a.Credits.SweepStale(ctx)
	switch {
	case err != nil:
		a.Hub.Log("err", "sweeper: %v", err)
	case n > 0:
		a.Hub.Log("ok", "sweeper: %d charge(s) stuck in_progress past %s → credits returned, status refunded", n, credsvc.ChargeTimeout)
		a.Hub.Changed()
	}
}

// ChargeTimeout is re-exported for the web layer (see credits/services).
const ChargeTimeout = credsvc.ChargeTimeout

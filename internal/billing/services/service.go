// Package services is the simulated billing logic: webhooks, grants per paid period, the mock provider, refunds, reconciliation.
package services

import (
	"bundle-credit-test/internal/billing"
	"bundle-credit-test/internal/platform/db"
	"bundle-credit-test/internal/platform/faults"
)

type Service struct {
	pool    *db.Pool
	store   billing.Store
	catalog billing.Catalog
	lic     billing.Licenses
	credits billing.Credits
	clock   billing.Clock
	log     billing.Log
	faults  *faults.Faults
}

func New(pool *db.Pool, store billing.Store, catalog billing.Catalog, lic billing.Licenses, credits billing.Credits,
	clock billing.Clock, log billing.Log, f *faults.Faults) *Service {
	return &Service{pool: pool, store: store, catalog: catalog, lic: lic, credits: credits, clock: clock, log: log, faults: f}
}

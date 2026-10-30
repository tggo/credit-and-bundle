// Package services is the credits logic: charge an AI action, refund it, grant and revoke lots, expire them.
package services

import (
	"bundle-credit-test/internal/credits"
	"bundle-credit-test/internal/platform/db"
)

type Service struct {
	pool   *db.Pool
	store  credits.Store
	lic    credits.Licenses
	prices credits.Prices
	ai     credits.AI
	clock  credits.Clock
	log    credits.Log
}

func New(pool *db.Pool, store credits.Store, lic credits.Licenses, prices credits.Prices, ai credits.AI, clock credits.Clock, log credits.Log) *Service {
	return &Service{pool: pool, store: store, lic: lic, prices: prices, ai: ai, clock: clock, log: log}
}

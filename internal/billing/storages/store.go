// Package storages keeps the simulated billing tables: subscriptions, payments, the webhook inbox and the mock provider.
package storages

import "bundle-credit-test/internal/platform/db"

type Store struct{ db db.Q }

func New(q db.Q) *Store { return &Store{db: q} }

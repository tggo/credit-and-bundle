// Package storages keeps wallets, lots, charges and the ledger in Postgres.
// Methods that take q run on the caller's pool or transaction.
package storages

import "bundle-credit-test/internal/platform/db"

type Store struct{ db db.Q }

func New(q db.Q) *Store { return &Store{db: q} }

// eligibleLotsSQL lists lots usable in an app, in spend order: narrow scope first, bonus before paid, soonest expiry first.
const eligibleLotsSQL = `SELECT l.id::text, l.account_id::text, l.label, l.remaining FROM credit_lots l
	WHERE l.account_id = $1 AND l.remaining > 0 AND l.expires_at > $3
	  AND (l.scope = 'platform' OR EXISTS (SELECT 1 FROM lot_scope_apps a WHERE a.lot_id = l.id AND a.application_id::text = $2))
	ORDER BY (l.scope = 'bundle') DESC, (l.kind = 'bonus') DESC, l.expires_at, l.created_at`

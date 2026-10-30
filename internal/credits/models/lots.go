package models

import "time"

// Grant is one lot to create from a payment.
type Grant struct {
	CustomerID, PaymentID, OfferingItemID, SubscriptionID string
	Label, Scope, Source                                  string
	Credits                                               int64
	Apps                                                  []string // scope "bundle": apps where the lot may be spent
	Expires, At                                           time.Time
}

// Lot is a lot row for views.
type Lot struct {
	Label, Scope, Apps, Kind, Source string
	Granted, Remaining               int64
	Expires                          time.Time
	Expired, Revoked                 bool // Revoked: taken back by a payment refund
}

// Usage is how much of a payment's lots was granted and is left (for the refund quote).
type Usage struct{ Granted, Remaining int64 }

// LedgerEntry is a ledger row for views.
type LedgerEntry struct {
	At        time.Time
	Type, Lot string
	Delta     int64
	Note      string
}

// ChargeRow is a charge row for views.
type ChargeRow struct {
	ID, App, Action, Status, Key string
	Amount                       int64
	At                           time.Time
}

// Flow is one edge of the credit flow: from a lot to where its credits went.
type Flow struct {
	Lot, Dest string
	Credits   int64
}

// LotLeft is a lot with the credits it has (or had, for zeroed lots).
type LotLeft struct {
	ID, AccountID, Label string
	Remaining            int64
}

// Entry is one ledger row to write.
type Entry struct {
	AccountID, LotID, ChargeID, Type, Note string
	Delta                                  int64
	At                                     time.Time
}

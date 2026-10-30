// Package models are the billing types: provider events, subscriptions, payments, refund quotes, reconciliation rows.
// Billing is the Billing team's domain in the real system; here it is simulated to drive the credits flows end to end.
package models

import "time"

// Event is what the payment provider sends us.
type Event struct {
	EventID     string    `json:"event_id"`
	Type        string    `json:"type"` // subscription.created | invoice.paid | invoice.failed | subscription.canceled | payment.refunded
	SubRef      string    `json:"sub_ref"`
	PaymentRef  string    `json:"payment_ref,omitempty"`
	CustomerID  string    `json:"customer_id"`
	OfferingID  string    `json:"offering_id"`
	AmountMinor int64     `json:"amount_minor,omitempty"`
	RefundMinor int64     `json:"refund_minor,omitempty"`
	PeriodStart time.Time `json:"period_start,omitzero"`
	PeriodEnd   time.Time `json:"period_end,omitzero"`
}

// Subscription is the row the webhook handler locks.
type Subscription struct {
	ID, Ref, CustomerID, OfferingID, Kind, Status string
	PeriodEnd                                     *time.Time
}

// Transitions are the allowed subscription moves; a terminal state has none.
var Transitions = map[string]map[string]bool{
	"created":  {"active": true, "canceled": true},
	"active":   {"active": true, "past_due": true, "canceled": true},
	"past_due": {"active": true, "canceled": true},
	"canceled": {},
}

// Paid is the latest succeeded payment of a subscription.
type Paid struct {
	ID, Ref, OfferingID string
	Amount              int64
	Start, End          time.Time
}

// RefundQuote is the partial refund: apps by unused time, credits by unspent amount.
type RefundQuote struct {
	PaymentRef           string
	Amount               int64
	AppsPart, CredPart   int64
	UnusedFrac, CredFrac float64
	Refund               int64
}

// ReconRow is one discrepancy between the provider and us.
type ReconRow struct {
	Kind, PaymentRef, Detail string
	Fixable                  bool
}

// ProviderTxn is the provider's record of a payment.
type ProviderTxn struct {
	Ref, SubRef, CustomerID, OfferingID, Status string
	Refunded                                    int64
	Created                                     time.Time
}

// SubView and WebhookRow are rows for the pages.
type SubView struct {
	ID, Ref, Offering, Kind, Status string
	End                             *time.Time
	Quote                           *RefundQuote
}

type WebhookRow struct {
	At                               time.Time
	ID, Type, SubRef, Status, Reason string
}

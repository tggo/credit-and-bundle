package models

import "time"

// BalanceSource is one lot in the balance breakdown.
type BalanceSource struct {
	Label       string    `json:"label"`
	Source      string    `json:"source"`
	Scope       string    `json:"scope"`
	Kind        string    `json:"kind"`
	Remaining   int64     `json:"remaining"`
	AppliesHere bool      `json:"applies_here"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Balance struct {
	ApplicationID string          `json:"application_id,omitempty"`
	Available     int64           `json:"available"`
	Total         int64           `json:"total"`
	BySource      []BalanceSource `json:"by_source"`
}

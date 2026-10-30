// Package models are the credits types: charges, balances, lots and the ledger.
package models

import "encoding/json"

// ChargeResult is the response body of POST /v1/credits/charges.
type ChargeResult struct {
	ChargeID  string          `json:"charge_id,omitempty"`
	Status    string          `json:"status"`
	Charged   int64           `json:"charged,omitempty"`
	Available int64           `json:"available"`
	Required  int64           `json:"required,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	Refunded  bool            `json:"refunded,omitempty"`
	Blocked   bool            `json:"blocked,omitempty"` // content policy refused; still charged, the provider did the work
}

// ChargeRequest is one AI action of a customer in an app.
type ChargeRequest struct {
	CustomerID, AppID, Action, Key, Input string
}

// Fail builds an error result.
func Fail(code string) ChargeResult { return ChargeResult{Status: "error", Error: code} }

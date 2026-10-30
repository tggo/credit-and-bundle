// Package models are the license types.
package models

import "time"

// Grant is one license to issue or extend for a paid period.
type Grant struct {
	SubscriptionID, CustomerID, OfferingItemID, AppID string
	Due                                               time.Time
}

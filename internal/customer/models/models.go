// Package models are the customer types.
package models

// Customer: ID is the uuid every other table references; Handle is the readable unique name used in URLs.
type Customer struct {
	ID, Handle, Email, Name string
}

// Short is "handle · first 8 of uuid" for display.
func (c Customer) Short() string {
	if len(c.ID) >= 8 {
		return c.Handle + " · " + c.ID[:8]
	}
	return c.Handle
}

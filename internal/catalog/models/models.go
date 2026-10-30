// Package models are the catalog types: what is sold and what an AI action costs.
package models

// Offering is a sellable product: one or more items (plans and credit packages) for one price.
type Offering struct {
	ID, Code, Name, Kind, Period string
	Price                        int64 // minor units
}

// Item is one position of an offering. Exactly one of AppID (plan item) or Credits (credit package item) is set.
type Item struct {
	ID, Type string
	AppID    *string
	Credits  *int64
	PackApp  *string // in-app credit pack: credits usable only in this app
	Validity *int64
	Weight   int64 // share of the offering price, minor units
}

// App is an application with its AI actions.
type App struct {
	ID, Code, Name, Vendor string
	Actions                []Action
}

// Action is an AI action of an app and its current price in credits.
type Action struct {
	Code  string
	Price int64
}

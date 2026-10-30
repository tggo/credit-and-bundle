package app_test

import "testing"

// Credits behaviour as specifications. Prices: appshop ai.generate 80, notes ai.summarize 20, cleanbox ai.scan 30.
// The Design+ bundle grants 10000 credits for appshop, retouch and notes; the 500 pack is spendable anywhere.

const (
	design = "Design+ bundle"
	pack   = "Credit pack 500"
)

func TestSpecs_Credits(t *testing.T) {
	runSpecs(t, "Credits", []spec{
		{
			Name:  "bundle credits are spent before the pack inside a bundle app",
			Given: []step{buy{"alice", "off_design"}, buy{"alice", "off_pack500"}},
			When:  charge{"alice", "appshop", "ai.generate", "a1"},
			Then: then{Status: 201,
				Ledger: []entry{{Type: "charge", Lot: design, Charge: "charge#1", Delta: -80}},
				Lots:   map[string]int64{design: 9920, pack: 500}},
		},
		{
			Name:  "an app without a license is refused and nothing is charged",
			Given: []step{buy{"alice", "off_design"}, buy{"alice", "off_pack500"}},
			When:  charge{"alice", "cleanbox", "ai.scan", "c1"},
			Then:  then{Status: 403, Lots: map[string]int64{pack: 500}},
		},
		{
			Name:  "outside the bundle only the pack is spent",
			Given: []step{buy{"alice", "off_design"}, buy{"alice", "off_pack500"}, buy{"alice", "off_cmm"}},
			When:  charge{"alice", "cleanbox", "ai.scan", "c2"},
			Then: then{Status: 201,
				Ledger: []entry{{Type: "charge", Lot: pack, Charge: "charge#1", Delta: -30}},
				Lots:   map[string]int64{design: 10000, pack: 470}},
		},
		{
			Name:  "a provider failure returns the credits to the same lot",
			Given: []step{buy{"alice", "off_design"}, aiFails{}},
			When:  charge{"alice", "notes", "ai.summarize", "f1"},
			Then: then{Status: 502,
				Ledger: []entry{
					{Type: "charge", Lot: design, Charge: "charge#1", Delta: -20},
					{Type: "refund", Lot: design, Charge: "charge#1", Delta: 20}},
				Lots: map[string]int64{design: 10000}},
		},
		{
			Name:  "content refused by the provider is still charged",
			Given: []step{buy{"alice", "off_design"}, aiBlocks{}},
			When:  charge{"alice", "notes", "ai.summarize", "b1"},
			Then: then{Status: 201,
				Ledger: []entry{{Type: "charge", Lot: design, Charge: "charge#1", Delta: -20}}},
		},
		{
			Name:  "a retry with the same key replays the stored result without charging again",
			Given: []step{buy{"alice", "off_design"}, charge{"alice", "appshop", "ai.generate", "k1"}},
			When:  charge{"alice", "appshop", "ai.generate", "k1"},
			Then:  then{Status: 200, Lots: map[string]int64{design: 9920}},
		},
		{
			Name:  "a renewal burns what is left of the old period and grants a fresh lot",
			Given: []step{buy{"alice", "off_design"}, charge{"alice", "appshop", "ai.generate", "e1"}},
			When:  days{31},
			Then: then{
				Ledger: []entry{
					{Type: "grant", Lot: design + " (2)", Delta: 10000},
					{Type: "expire", Lot: design, Delta: -9920}},
				Lots: map[string]int64{design: 0, design + " (2)": 10000}},
		},
	})
}

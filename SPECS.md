# Credits

## ✓ bundle credits are spent before the pack inside a bundle app

- Given alice buys off_design
- And alice buys off_pack500
- **When** alice runs ai.generate in appshop (key a1)
- **Then** the response is 201
- ledger: charge -80 on "Design+ bundle" for charge#1
- "Credit pack 500" has 500 left
- "Design+ bundle" has 9920 left

## ✓ an app without a license is refused and nothing is charged

- Given alice buys off_design
- And alice buys off_pack500
- **When** alice runs ai.scan in cleanbox (key c1)
- **Then** the response is 403
- the ledger gets no new entries
- "Credit pack 500" has 500 left

## ✓ outside the bundle only the pack is spent

- Given alice buys off_design
- And alice buys off_pack500
- And alice buys off_cmm
- **When** alice runs ai.scan in cleanbox (key c2)
- **Then** the response is 201
- ledger: charge -30 on "Credit pack 500" for charge#1
- "Credit pack 500" has 470 left
- "Design+ bundle" has 10000 left

## ✓ a provider failure returns the credits to the same lot

- Given alice buys off_design
- And the AI provider returns 5xx
- **When** alice runs ai.summarize in notes (key f1)
- **Then** the response is 502
- ledger: charge -20 on "Design+ bundle" for charge#1
- ledger: refund +20 on "Design+ bundle" for charge#1
- "Design+ bundle" has 10000 left

## ✓ content refused by the provider is still charged

- Given alice buys off_design
- And the AI provider refuses the content
- **When** alice runs ai.summarize in notes (key b1)
- **Then** the response is 201
- ledger: charge -20 on "Design+ bundle" for charge#1

## ✓ a retry with the same key replays the stored result without charging again

- Given alice buys off_design
- And alice runs ai.generate in appshop (key k1)
- **When** alice runs ai.generate in appshop (key k1)
- **Then** the response is 200
- the ledger gets no new entries
- "Design+ bundle" has 9920 left

## ✓ a renewal burns what is left of the old period and grants a fresh lot

- Given alice buys off_design
- And alice runs ai.generate in appshop (key e1)
- **When** 31 days pass
- ledger: grant +10000 on "Design+ bundle (2)"
- ledger: expire -9920 on "Design+ bundle"
- "Design+ bundle" has 0 left
- "Design+ bundle (2)" has 10000 left


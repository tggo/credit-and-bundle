# Manual test plan — production (https://credit-and-bundle.ihatebot.com)

Run in a real browser (Chrome). Each check: what to do → what must be true. Mark `[x]` pass, `[!]` fail (with a
finding below), `[ ]` not run. Anything found during a run that the plan did not cover goes under **Added during
the run**, then becomes a numbered check.

Before the run: `reset` (password: the `RESET_PASSWORD` from `/opt/credit-and-bundle/.env`) so data starts from the seed.

## 0. Global
- [!] 0.1 Every nav link (Simulator, Webhooks, Reconciliation, Board, Diagrams, Data, API, Tables, Migration, Testing) opens with no console errors. (Diagrams had errors — F8, F9, fixed and rechecked)
- [x] 0.2 Clock shows the simulated date; +1 / +15 / +31 days move it and the log says so.
- [x] 0.3 Customer switch alice ↔ bob changes the state panel to that customer.
- [x] 0.4 reset asks for a password; wrong password refuses, the real `RESET_PASSWORD` resets.
- [x] 0.5 Live log (SSE) updates without reload; no `ERROR` lines during the happy path.
- [x] 0.6 Static assets carry `?v=`; a second tab sees updates from the first (SSE). (only `?v=` checked)

## 1. Simulator — purchases and grants
- [x] 1.1 Catalog scrolls inside its own box; page below it stays visible.
- [x] 1.2 Buy Design+ bundle → licenses for every bundle app; bundle credit lot with scope = bundle apps.
- [x] 1.3 Buy Credit pack 500 → unscoped lot of 500.
- [x] 1.4 Buy CleanBox monthly (no credits) → license only, no lot.
- [x] 1.5 Buy CleanBox + AI → license + 300 credits scoped to CleanBox.

## 2. Simulator — spending
- [x] 2.1 AI in a bundle app → bundle lot is spent first, the pack stays.
- [x] 2.2 AI in an app without license → 403, no charge.
- [x] 2.3 Not enough credits → 402, balance unchanged.
- [x] 2.4 Race ×5 → exactly as many 201 as credits allow, rest 402; balance never negative.
- [x] 2.5 Retry same key → one charge; duplicates 409 / replay of stored result.
- [x] 2.6 Provider 5xx switch → credits returned to the same lots.
- [x] 2.7 Content block switch → behaviour matches the design (charged or refunded as documented).

## 3. Simulator — time, refund, cancel
- [x] 3.1 +15 days, refund the bundle → apps and credits returned pro rata; log shows the amounts.
- [x] 3.2 +31 days → renewal grants a new lot; old lot expires.
- [x] 3.3 Cancel subscription → no renewal at the next period.

## 4. Webhooks
- [x] 4.1 Provider re-sends the same invoice.paid → duplicate by event_id, nothing granted twice.
- [x] 4.2 Same payment_ref under a new event_id → duplicate by payment_ref.
- [x] 4.3 invoice.paid before subscription.created → handled, rights granted once.
- [x] 4.4 Late subscription.created → no regression of state.
- [x] 4.5 past_due → active allowed; terminal states stay terminal.

## 5. Reconciliation
- [x] 5.1 After reset: Checked N, no discrepancies, totals equal.
- [!] 5.2 Lose invoice.paid + buy pack → missing_ours; Replay webhook → row gone, credits granted. (F1, fixed and rechecked)
- [x] 5.3 Payment only we know → missing_provider, manual review.
- [x] 5.4 Refund without webhook → status_mismatch; Replay webhook → rights revoked, row gone.
- [x] 5.5 Page explains itself: what reconciliation is, what each button breaks, what each row means.

## 6. Board
- [x] 6.1 Current / changed / charge-flow tabs render; edges attach to fields.
- [ ] 6.2 (not run: would move the shared board layout) Drag a table, reload → position kept.
- [ ] 6.3 (not run: needs two real browsers) Two tabs → cursor and moves visible in the other tab.

## 7. Diagrams
- [!] 7.1 Every diagram renders, not stretched, readable font. (sankey raw text, no pan-zoom — F8, F9, fixed and rechecked)
- [!] 7.2 Click a diagram → editor; edit → live preview; click in SVG → jumps to the line. (state boxes did not jump — F10, fixed)

## 8. Data
- [x] 8.1 Table list; open a table → rows + structure (PK/FK/UQ/checks/indexes/DDL) + descriptions.
- [x] 8.2 Data reflects actions just done in the simulator.

## 9. Slides (removed 2026-10-27 — replaced by /tables, /migration, /testing)
- [x] 9.1 ~~15 slides~~ — n/a, `/slides` removed.

## Findings

Run 1 — 2026-10-26, Chrome, production.

| # | Check | Observed | Status |
|---|---|---|---|
| F1 | 5.2, 4.3 | Buttons sent an offering code (`off_pack500`), lookup accepted only uuid → `invalid input syntax for type uuid` | fixed: lookup by id or code |
| F2 | 0.4 → 1.x | After reset seeds get new uuids; open tabs keep old ids → every Buy logs `no rows in result set` | fixed: reset publishes SSE `reload`, every open page reloads |
| F3 | 2.7 | Content block returns 201 and charges; log looked like a normal success | fixed: result has `blocked: true`, log says "still charged, the provider did the work" |
| F4 | 3.1 | One-time Credit pack showed "Cancel at period end" and "apps $0.00×96%" in the refund quote | fixed: no cancel on one_time, apps part hidden when 0 |
| F5 | 3.3 | Canceled subscription showed "until Nov 25" — reads like it ended | fixed: "access until" |
| F6 | 3.2 | Renewal log did not say which subscription | fixed: provider ref + next period date |
| F7 | 4.x | Webhooks page "as bob" listed every customer's subscriptions with no hint | fixed: headings say "all customers" |
| F8 | 7.1 | Live credit flow showed raw `sankey-beta` text: `draw is not defined` (the htmx-swap hook lived in diagrams.js, `draw` in the module) | fixed: hook moved into the module |
| F9 | 7.1 | svg-pan-zoom 3.6.1 404 on cdnjs → no zoom/pan on any diagram | fixed: jsDelivr URL |
| F10 | 7.2 | Clicking a state box in the states editor did nothing (implicit states have no `data-source-line`) | fixed: fallback jumps to the first line naming the clicked text |
| F11 | 1.5 | A subscription lot (CleanBox + AI) is labelled scope `bundle` | fixed (wording): column shows "only: CleanBox" / "all apps", raw scope below; schema value is by design |
| F12 | 3.1/3.2 | A lot zeroed by refund later shows "· expired", not "revoked by refund" | fixed: "· revoked (refund)" from the ledger revoke row; checked on prod |
| F13 | 5.3 | `missing_provider` has no action besides "manual review" and no customer shown | open, by design (manual review); run 5.3 last |
| F14 | 6.1 | Board is blank for ~2 s while esm.sh modules load, no loading text | fixed: "Loading board…" placeholder |
| F15 | 3.1 | Refund quote showed "credits $0.00×0%" for subs without credits; Refund button shown for a $0 quote (cancels + revokes for nothing) | fixed (review) |
| F16 | 7.2 | F10 fallback read the `<rect>` text (empty) → state boxes still did not jump | fixed (review): label inside the box is used; rechecked all four states |
| F17 | 7.1 | Sankey pan-zoom not in try/catch | fixed (review) |
| F18 | slides | Slide + design doc said `$24×50% + $6×98% = $17.91` — arithmetic is $17.88 | fixed: measured: 400 spent → $17.76 |
| F19 | 4.5→3.1 | Refund after past_due revokes ALL lots/licenses of the sub, quote prices only the latest payment (`apply.go:105`) | fixed: lots revoked by payment_id; licenses cut to the end of the latest still-paid period (`TestFlow_RefundAfterPastDueKeepsEarlierPeriod`) |
| F20 | 2.5 | Replay runs after license/price checks: replay after a revoke returns 403, not the stored result (`charge.go:23`) | fixed: stored key replays before the license/price checks (`TestIdempotency_ReplayAfterLicenseRevoked`) |
| F21 | — | Crash between debit and CompleteCharge leaves `in_progress` → 409 forever (no sweeper) | fixed: sweeper refunds `in_progress` older than `ChargeTimeout` = 2 min (`TestSweeper_*`) |
| F22 | demo 7→8 | Sweeper on +31 days refunded a crashed charge into a lot that had just expired: lot "expired" but holding 15 | fixed: SweepStale runs Expire after refunds (`TestSweeper_RefundIntoExpiredLotIsBurned`) |
| F23 | review | A charge in_progress drained a lot to 0; payment.refunded skipped lots at 0 (no revoke row); the charge then failed / was swept and its credits came back into the refunded lot | fixed: `credit_lots.revoked_at` set on every lot of the payment, even at 0; burn checks `revoked_at` (`TestRefund_IntoDrainedRevokedLotIsBurned`) |

## Added during the run

- [x] 0.7 After reset, an already open page reloads by itself and Buy works (F2).
- [x] 1.6 One-time purchases have no cancel button; refund quote shows only parts that apply (F4).
- [x] 2.8 Content-block result is visible in the log as charged (F3).
- [x] 7.3 Live credit flow sankey renders after the htmx swap (F8).
- [x] 7.4 Pan/zoom controls present on every PlantUML diagram (F9).
- [x] 7.5 Click-to-line works for state boxes, transitions and sequence participants (F10).
- [ ] 7.6 No console errors on the Diagrams page (one htmx `Event` error seen during a deploy restart — recheck).

## Added by review (Fable, 2026-10-26) — run 2

Exact values assume a fresh reset and this order. Bob track:

- [ ] 2.9 bob buys App Shop monthly → Use AI App Shop → `402 error insufficient_credits (available here: 0)`; no Charges row.
- [ ] 2.10 bob buys App Shop Pro → lot `only: App Shop` 1000/1000; App Shop `1000 of 1000`, other apps `0 of 1000`.
- [ ] 2.11 bob buys Credit pack 500 → App Shop `1500 of 1500`; 12 × 80 spent → Pro lot **40**, pack **500** (narrow scope first).
- [ ] 2.12 Use AI App Shop → one Charges row (80), two ledger `charge` rows `-40 Pro`, `-40 pack`; pack **460**, Pro **0**.
- [ ] 2.13 Race ×5 → 5×201, pack 60; Race ×5 → `5×402 · balance 60 → 60 … consistent`; Use AI → `402 (available here: 60)`.
- [ ] 2.14 Retry same key at balance 60 → `3×402`, then `402`, `402`; no `retry_` row in Charges.
- [ ] 2.15 +1 day, buy pack again → Use AI → older pack drained first (`-60` old, `-20` new); new pack 480.
- [ ] 2.16 provider 5xx + Retry same key → `1×502, 2×409`, replay `502 refunded`, `422` on different body; ledger charge + refund on the same lot.
- [ ] 2.17 Fault switches are global: set 5xx as alice, Use AI as bob → bob gets 502; alice's next call 201.
- [ ] 2.18 Ledger invariant: on Data, `credit_lots.remaining` = Σ `credit_ledger.delta` per lot; sankey right side sums to granted.
- [ ] 3.4 After refund: bundle apps License `none`, Use AI → `403 no_active_license`, pack intact, Reconciliation `No discrepancies`.
- [ ] 3.5 +31 days: exactly one `renewal: provider charged sub_… for X (next period from …)` per active recurring sub; none for canceled.
- [ ] 3.6 One-time pack with 20 left → `refund now: $0.20 = credits $5.00×4%`, no Cancel; fully spent pack → no Refund button (F15).
- [ ] 4.6 Fresh customer: Duplicate → `no payments yet`, Late created → `no subscriptions yet`, Past-due → `no active recurring subscription`.
- [ ] 5.6 Run 5.3 (phantom) last — its row stays until reset.
- [ ] 8.3 `GET /api/v1/credits/balance` (`X-Customer-Id`): no `application_id` → available == total; with it → other apps' lots `applies_here: false`; expired and empty lots absent.
- [ ] 0.8 Set "Lose next invoice.paid", reset, buy → webhook arrives (reset clears switches).
- [x] 9.2 ~~Slides at the projector resolution~~ — n/a, `/slides` removed.

Rewrites of weak checks: 2.7 expected = `201 completed content blocked by policy — still charged…`, balance −price, no refund row.
3.1 expected (400 spent from Design+, day 15) = `apps $24.00 × 50% unused + credits $6.00 × 96% unspent = $17.76`, ledger `revoke -9600`,
lot `· revoked (refund)`, sub `canceled · access until <buy+30>`. 2.4 must say `consistent`, never `MISMATCH`.

Before the call: open Board and Diagrams once to warm the CDNs (esm.sh, jsDelivr, cdnjs).

## Added 2026-10-26 evening — replay-first, sweeper, in-app pack, two actions per app

- [x] 10.1 Every app row shows two AI actions with their own prices (App Shop: ai.generate 80, ai.upscale 40; Face Retouch: ai.retouch 50, ai.bg_remove 15; Notes AI: ai.summarize 20, ai.translate 35; CleanBox: ai.scan 30, ai.explain 10); each action's Use AI charges its own price.
- [x] 10.2 bob buys App Shop monthly + App Shop credits 300 → lot `only: App Shop`, 300; App Shop `300 of 300`, other apps `0 of 300`.
- [x] 10.3 In-app pack + platform pack 500: Use AI in App Shop takes from the in-app pack first (narrow scope), the platform pack stays 500. (checked with ai.upscale: in-app 300→260, platform pack stays 500)
- [x] 10.4 "Next AI call → process dies after debit", Use AI → log `500 in_progress simulated_crash`, balance −price; Retry same key on it is not possible from the UI, so check: Charges row `in_progress`.
- [x] 10.5 Within ~2 min 15 s (or on +1 day) → log `sweeper: 1 charge(s) stuck in_progress past 2m0s → credits returned`; Charges row `refunded`; ledger `refund … stale in_progress`; balance back.
- [ ] 10.6 Replay after refund: covered by `TestIdempotency_ReplayAfterLicenseRevoked` (UI has no way to resend an old key).

## Tech debt

- [ ] TD-1 Grace period: on `invoice.failed` the subscription goes `past_due`, but licenses and the bundle lot still end at the paid period end (`offerings.grace_days` is not read). Billing owns retries and the cancel decision; License/Credits own extending `due_date` by `grace_days` and keeping the bundle lot alive during grace. Test to add: past_due → access and credits live `grace_days` → no payment: 403 + `expire`; paid: next period without a gap.

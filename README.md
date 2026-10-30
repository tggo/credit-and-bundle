# Credits & Bundles — MVP

Working model of an app marketplace: bundles of apps + credit packages, one wallet with
scoped lots, charges for AI actions through a gateway, webhooks from a mock payment provider, refunds,
expiry, reconciliation. The design doc is kept outside this repo.

Stack: Go 1.27, Fiber v2, PostgreSQL 17 (Docker), server-side templates + HTMX, live updates over SSE,
PlantUML diagrams rendered to SVG.

## Run

```bash
docker compose up -d                        # Postgres on localhost:55433
go build -o bct ./cmd/server && ./bct       # http://localhost:8095
```

Every start recreates the schema and seed (`internal/platform/schema/schema.sql`); an older schema is replaced automatically. `KEEP_DATA=1 ./bct` keeps the data.
`ADDR`, `DATABASE_URL` override defaults.

## Layout

Same approach as feedql: `internal/<domain>/` with `iface.go` (interfaces it needs and offers), `models/` (types only),
`storages/` (Postgres), `services/` (logic over the iface). Files stay small (largest non-test file ~190 lines).

```
cmd/server            main: pool → app → web
internal/
  catalog/            offerings, items, plans, credit packages, AI action prices
  customer/           customers (uuid id + readable handle)
  license/            licenses per (subscription, plan item)
  credits/            wallet, lots, charges, append-only ledger; services: charge, refund, grant/revoke/expire, views
  billing/            simulated Billing team: provider, webhooks, renewals, refund quote, reconciliation
  board/              shared model boards (scenes + revision)
  platform/           db (Q + InTx), clock, sse hub, schema, faults, ids, aimock
  app/                composition root (+ integration tests)
  web/                handlers, templates, static (pages, fragments, sim, API, boards, data viewer)
  archcheck/          tests that enforce the layering
```

Rules (`internal/archcheck`): models import nothing; storages import only their own models and platform; services use
their own iface and models, never their own storages; another domain is reached only through its models and the caller's
iface (billing hands grants to License and Credits inside its own transaction via `db.Q`); platform imports only platform;
no domain imports app or web. A forbidden import fails `go test`.

Every id is a `uuid` except `credit_ledger.id`, a `bigserial` (journal order); `customers.handle`, `applications.code`, `offerings.code` etc. are separate UNIQUE columns for
seeds and URLs. `TestIDsAreUUIDs` checks it.

## Pages

| Page | What it shows |
|---|---|
| `/` Simulator | Catalog, buy as alice/bob, per-app balance (what each app sees), subscriptions with live refund quote, lots, ledger, charges, live log |
| `/webhooks` | Webhook inbox: applied / duplicate / parked / rejected, subscription states, scenarios |
| `/reconcile` | Provider transactions vs our payments by `payment_ref`, discrepancy kinds, replay fix |
| `/board` | Editable React Flow boards (changed model, current model, charge flow): drag blocks, edit text, draw arrows; saved on the server, survive resets; “Regenerate from model” discards edits |
| `/data` | Read-only view of every Postgres table, grouped by service, filter by customer, live |
| `/diagrams` | Live credit flow (sankey from the ledger) + PlantUML sequence/state diagrams with zoom and pan |
| `/api` | Credits API contract: charge and balance, errors, curl examples |
| `/tables` | Every table explained, field by field |
| `/migration` | Migration plan: 6 steps, SQL, gates, rollback |
| `/testing` | Testing strategy (unit, integration, concurrency, load, edge cases) and monitoring |

Header: simulated clock (+1 / +15 / +31 days triggers renewals and lot expiry), customer switch, reset.

## What the scenarios prove

- **Offerings:** a product can be a plain subscription (license only), a subscription with monthly credits (CleanBox + AI, App Shop Pro: credits usable only in that app), a bundle (apps + credits) or a pure credit pack (platform-wide).
- **Scope:** Design+ credits are spent only in Design+ apps; the platform pack works everywhere. Spend order: bundle scope → bonus → soonest expiry.
- **License check:** AI in an app without an active license → 403.
- **Race:** N devices spend the same balance at once → exactly as many succeed as the balance allows, the rest 402. The account row is locked first (`SELECT … FOR UPDATE`), then lots are debited; locking after the charge insert deadlocks because the FK takes a share lock on the same row.
- **Idempotency:** same key 3× at once → one charge, copies 409; after completion → 200 replay; same key, other body → 422. A known key is replayed before the license/price checks, so a retry after a refund still gets its stored 200.
- **Stuck charges:** "Next AI call → process dies after debit" leaves the charge `in_progress` (retries 409). The sweeper (every 15 s, also on +days) refunds `in_progress` older than 2 min, exactly once even with several instances.
- **In-app pack:** "App Shop credits 300" — credits bought inside one app, usable only there (`credit_packages.application_id`). Every app has two AI actions with different prices.
- **Provider failure:** 5xx → credits return to the same lots (`refund` ledger entries), response 502. Content-policy block is charged.
- **Partial refund:** apps part × unused time + credits part × unspent share; remaining bundle lots revoked, licenses revoked.
- **Renewal / expiry:** advancing the clock charges recurring subscriptions (new lot per period) and burns expired lots (`expire` entry).
- **Webhooks:** `invoice.paid` before `subscription.created` is parked and retried, `payment.refunded` before its `invoice.paid` too; duplicates by `event_id` and by `payment_ref`; late `subscription.created` after cancel is rejected; `invoice.failed` → past_due → paid → active.
- **Reconciliation:** lost webhook (customer paid, no rights) → replay; payment only on our side → manual review; provider refund without webhook → replay refund.

## API (as in the design doc)

`X-Customer-Id` stands in for the license token. Both it and `application_id` are uuids (from `/data`: `customers`, `applications`); seeds get new uuids on every reset.

```bash
curl -X POST localhost:8095/api/v1/credits/charges \
  -H "X-Customer-Id: $CUSTOMER" -H 'Idempotency-Key: k1' -H 'Content-Type: application/json' \
  -d "{\"application_id\":\"$APP\",\"action_code\":\"ai.generate\",\"input\":{\"prompt\":\"cat\"}}"

curl "localhost:8095/api/v1/credits/balance?application_id=$APP" -H "X-Customer-Id: $CUSTOMER"
```

## Simplifications

One process plays all services; the mock provider writes `provider_txns` and calls the webhook handler
in-process. AI provider is a 250 ms sleep. No auth, taxes, currencies beyond USD, or vendor payouts.

## Tests

Integration tests (`internal/app/*_test.go`) against the real Postgres in Docker (database `bundlecredit_test`, created once with
`docker exec bundle-credit-test-db-1 psql -U bc -d bundlecredit -c "CREATE DATABASE bundlecredit_test"`):

```bash
go test -race -count=1 ./...
```

| Test | What it proves |
|---|---|
| `TestRace_OneBalanceManyDevices` | 500 credits, 20 parallel charges of 80 → exactly 6×201 + 14×402, 20 left |
| `TestRace_ManyCustomersNoDeadlock` | 4 streams × 10 parallel charges across two customers → all 201, no 500/deadlock |
| `TestRace_ChargeSpanningTwoLots` | one charge takes 30 from the bundle lot and 50 from the pack, two ledger entries |
| `TestIdempotency_SameKeyConcurrent` | 10 copies of one key → one charge; replay → 200 stored result; other body → 422 |
| `TestIdempotency_402IsNotStored` | 402 is not remembered; the same key succeeds after a top-up |
| `TestIdempotency_ReplayAfterLicenseRevoked` | retry of a stored key after a refund → 200 stored result; a new key → 403 |
| `TestSweeper_CrashAfterDebitReturnsCredits` | crash after debit → 500, retry 409; sweep before timeout does nothing; after it 4 parallel sweeps refund once; late refund is a no-op; retry → 502 |
| `TestFlow_RefundAfterPastDueKeepsEarlierPeriod` | refund of the renewal paid after past_due revokes only that payment's lot; the earlier period keeps credits and access until its end |
| `TestFlow_InAppPackOnlyForItsApp` | in-app pack lot is scoped to its one app: other app 402, own app 201 |
| `TestFlow_ScopeAndSpendOrder` | bundle credits only in bundle apps; no license → 403; outside app spends only the pack |
| `TestFlow_ProviderFailureRefundsSameLots` | 5xx → 502 + refund entry, balance restored; content block is charged |
| `TestFlow_PartialRefund` | day 15: $24×50% + $6×96%; licenses and remaining lots revoked |
| `TestFlow_RenewalAndExpiry` | +31 days → second payment, old lot expires (9920), fresh lot 10000 |
| `TestWebhooks_OutOfOrderAndDuplicates` | parked paid applied after created; 10 concurrent duplicates → 1 payment, 1 lot |
| `TestWebhooks_TerminalStateAndPastDue` | failed → past_due → paid → active; late created after cancel rejected |
| `TestFlow_SubscriptionWithAndWithoutCredits` | a plain subscription gives a license and no lot; one with credits gives a lot scoped to its app |
| `TestSweeper_RefundIntoExpiredLotIsBurned` | a sweeper refund into a lot that expired meanwhile is burned in the same pass |
| `TestRefund_IntoRevokedLotIsBurned` | a charge refunded after its payment was refunded does not bring the revoked credits back |
| `TestRefund_IntoDrainedRevokedLotIsBurned` | same, when the charge had taken the lot to exactly 0, so the payment refund found nothing to zero (`credit_lots.revoked_at`) |
| `TestRefund_RacesChargesWithoutDeadlock` | 200 staggered charges, half failing and refunding while the rest debit the same lot: no deadlock, ledger holds |
| `TestWebhooks_RefundBeforePaidIsParked` | `payment.refunded` before its `invoice.paid` is parked, then applied after the payment lands |
| `TestIDsAreUUIDs` | every id / *_id column is uuid (the `bigserial` ledger id is the one exception) |
| `TestEveryTableAndKeyColumnIsDescribed` | every table and key column has a `COMMENT ON` description |
| `TestSpecs_Credits` | credits behaviour as Given/When/Then specifications (below) |

### Specifications

`internal/app/specs_*_test.go` describe behaviour as data: **Given** steps that already happened, **When** one step,
**Then** what it produced: the response status, the new ledger rows and the lots it left. Charges are named
`charge#1`, `charge#2` in the order they appear, and lots by their label without the period date (a second lot of
the same name is `Design+ bundle (2)`), so a spec has no uuids. A failure names the field, e.g.
`Ledger[1].Delta: want -9900, got -9920`. The same specs print as plain text for review:

```bash
go test ./internal/app -run TestSpecs -specdoc="$PWD/SPECS.md" -specreport="$PWD/specs.html"
```

`-specreport` also records every SQL statement a When step runs and explains it again with `enable_seqscan = off`.
Postgres then takes any index that can serve the query, so a Seq Scan left in the plan means no index fits it;
the report lists those per table (on test-sized tables a plain EXPLAIN would always pick Seq Scan and hide it).
| `TestBoardShowsEveryDesignTable` | every design table is on the model board |
| `TestBoardFieldsMatchSchema` | board columns equal schema columns (known differences listed in `boardHidden`) |
| `TestBoardCardinalityMatchesSchema` | the board's nullable-FK ends equal the schema's |
| `TestLayerRules` · `TestDomainsHaveIface` · `TestNoUnknownTopLevelPackages` · `TestAllowedTable` (`internal/archcheck`) | the layering rules: a forbidden import fails the build |
| `TestReconcile_LostWebhookFound` | lost webhook found as missing_ours, replay grants credits, recon clean |

Every test also checks the ledger invariant: `lot.remaining == SUM(ledger.delta)` and never negative.
Mutation check: moving the account lock after the charge insert makes both race tests fail with
deadlocks (`500` on 18 of 20 requests (measured 2026-10-26, 5 runs: 18, 18, 18, 18, 17 — `TestRace_OneBalanceManyDevices` with the lock moved after the insert)), so the tests do guard the lock order.
Same check, 2026-10-26, for the other two guards: running the stored-key replay after the license check makes `TestIdempotency_ReplayAfterLicenseRevoked` fail; dropping `AND status='in_progress'` from the refund claim makes `TestSweeper_CrashAfterDebitReturnsCredits` fail (second refund adds 50 credits); removing the post-sweep `Expire` makes `TestSweeper_RefundIntoExpiredLotIsBurned` fail. 2026-10-27: without the account lock in `RefundCharge`, `TestRefund_RacesChargesWithoutDeadlock` fails with ~100 of 200 charges answering 500 (deadlock); without the burn of dead lots `TestRefund_IntoRevokedLotIsBurned` fails; rejecting an unknown payment instead of parking fails `TestWebhooks_RefundBeforePaidIsParked`; reading the revoked state from `revoke` ledger rows instead of `credit_lots.revoked_at` fails `TestRefund_IntoDrainedRevokedLotIsBurned` (10000 credits come back).

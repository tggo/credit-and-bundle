# Credits & Bundles MVP — working rules

Go + Fiber + Postgres + HTMX/SSE simulator of an app marketplace. Public: https://credit-and-bundle.ihatebot.com
(`deploy/deploy.sh`). The design doc is kept outside this repo.

## Layout

`internal/<domain>/{iface.go, models/, storages/, services/}` (as feedql). Domains: catalog, customer, license,
credits, billing (simulated Billing team), board. Plus `platform/` (db, clock, sse, schema, faults, ids, aimock,
plantuml), `app/` (wiring + integration tests), `web/` (handlers, templates, static), `cmd/server`.
`internal/archcheck` enforces the layering; a forbidden import fails `go test`. Keep files small (< ~200 lines).

## One model, many views — change them together

The data model is shown in several places. A schema change is not done until every one of them says the same thing:

| What | Where | Guard |
|---|---|---|
| Tables, columns, keys | `internal/platform/schema/schema.sql` | integration tests |
| Table and column descriptions | `internal/platform/schema/comments.sql` (`COMMENT ON`, applied on every start) | `TestEveryTableAndKeyColumnIsDescribed` |
| Model board (current / changed / charge flow) | `internal/web/static/board.js` → `DEFS` (bump `BOARD_VERSION` so saved boards regenerate) | `TestBoardShowsEveryDesignTable`, `TestBoardFieldsMatchSchema` (every column; known differences listed in `boardHidden`) |
| Sequence and state diagrams | `internal/web/diagrams/*.puml` (fallback SVGs: `diagrams/render.sh`) | manual |
| Docs pages (tables, migration, testing) | `internal/web/templates/{tables,migration,testing}.html` | manual |
| Design doc | the design doc (kept outside this repo) | manual |
| README tables (layout, scenarios, tests) | `README.md` | manual |

Checklist for a schema change:
1. `schema.sql` — the table/column; every id and `*_id` is `uuid` (`TestIDsAreUUIDs`), except the `bigserial` `credit_ledger.id`; readable codes are separate UNIQUE columns.
2. `comments.sql` — `COMMENT ON TABLE` for a new table, `COMMENT ON COLUMN` for anything non-obvious.
3. `board.js` `DEFS.changed` — the table with its rows (`+` new, `~` changed, `-` removed → where it moved), relations
   (field-level FK edges are derived from `<table>_id` names); bump `BOARD_VERSION`.
4. `.puml` diagrams if a flow changes; run `internal/web/diagrams/render.sh` (needs `docker compose up -d plantuml`).
5. Storage/service code + an integration test for the new behaviour.
6. `README.md`, the docs pages (`/tables`, `/migration`, `/testing`) and the design doc.
7. `go test -race ./...`, then deploy and look at Board, Diagrams and Data on production.

Schema changes reset the simulator data: `schema.Seeded` checks a marker column; change the marker when the schema
is not compatible with existing rows. Descriptions (`comments.sql`) never need a reset.

## Running

```bash
docker compose up -d                        # Postgres :55433, PlantUML server :18095
go build -o bct ./cmd/server && ./bct       # http://localhost:8095 (KEEP_DATA=1 keeps data)
go test -race -count=1 ./...                # needs the bundlecredit_test database
deploy/deploy.sh                            # DEPLOY_HOST=<ssh host>; static URLs carry ?v=<build> so Cloudflare never serves old JS/CSS
```

## Specifications

New or changed credits behaviour starts as a row in `internal/app/specs_*_test.go` (Given / When / Then), written
before the code. Then holds only what is visible from outside (status, new ledger rows, lot balances); keep the
names symbolic (`charge#N`, lot label without date). Regenerate `SPECS.md` (see README) when specs change.

## Specifications

New or changed credits behaviour starts as a row in `internal/app/specs_*_test.go` (Given / When / Then), written
before the code. Then holds only what is visible from outside (status, new ledger rows, lot balances); keep the
names symbolic (`charge#N`, lot label without date). Regenerate `SPECS.md` (see README) when specs change.

## Gotchas

- Lock order in a charge: `credit_accounts FOR UPDATE` first, then the charge insert. The other order deadlocks
  (FK key-share lock); the race tests fail if it changes. Refund, revoke and expire take the same account lock first
  (expire: all affected accounts in id order) — `TestRefund_RacesChargesWithoutDeadlock` deadlocks without it.
- PlantUML server measures text with DejaVu Sans; the page serves the same font (`static/fonts`) — keep them equal,
  or diagram text stretches. Server SVGs carry `preserveAspectRatio="none"`; the page overrides it.
- Reset on the public deploy asks for `RESET_PASSWORD` (in `/opt/credit-and-bundle/.env`).
- A charge left `in_progress` (process died between debit and complete) is refunded by the sweeper (`App.RunSweeper`,
  every 15 s, and on +days) after `credits.ChargeTimeout` = 2 min. Refund and complete only move a charge out of
  `in_progress` with a conditional UPDATE — keep that, or two sweepers return credits twice (`TestSweeper_…` catches it).
- A stored idempotency key is replayed before the license/price checks (`TestIdempotency_ReplayAfterLicenseRevoked`).

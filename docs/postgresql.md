# PostgreSQL operations

AuctionLab requires PostgreSQL **18.x** as a separate service. The Go binary embeds the frontend and still builds with `CGO_ENABLED=0`; it does not bundle PostgreSQL. Offline operation needs the app, local database process, and media directory. Dependency downloads and initial provisioning require connectivity.

Provision a PostgreSQL 18 instance using your platform package or an existing local container runtime. Bind development PostgreSQL to loopback. For a disposable container, set `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` through an untracked environment file, then run:

```sh
docker run --detach --name auctionlab-pg18 --env-file .env.postgres \
  --publish 127.0.0.1:5432:5432 postgres:18
```

The initialized container administrator is for provisioning only. In `psql`, create separate app/test owners and databases (choose passwords with `\password`, rather than embedding secrets in SQL history):

```sql
CREATE ROLE auctionlab_app LOGIN;
\password auctionlab_app
CREATE DATABASE auctionlab OWNER auctionlab_app;
CREATE ROLE auctionlab_test LOGIN;
\password auctionlab_test
CREATE DATABASE auctionlab_test OWNER auctionlab_test;
```

Only use the test role/database for disposable fixtures; it needs schema-creation privileges in that database. Do not point tests at your operational database. Configure credentials in your shell or secret manager, not the repository. Local URLs use loopback and explicit `sslmode=disable`; remote URLs must specify the appropriate verified TLS settings. The app does not weaken the supplied TLS configuration.

```sh
export DATABASE_URL='postgresql://<app-role>:<password>@127.0.0.1:5432/auctionlab?sslmode=disable'
export DATABASE_SCHEMA=auctionlab
export TEST_DATABASE_URL='postgresql://<test-role>:<password>@127.0.0.1:5432/auctionlab_test?sslmode=disable'
make dev
make test
make test-race
make build
make loadtest-20
make loadtest-tunnel
```

Replace the angle-bracket placeholders privately. `DATABASE_URL` is required; `DATABASE_SCHEMA` defaults to `auctionlab` and must be a dedicated lowercase SQL identifier (not `public` or `pg_*`). Runtime `DB_PATH` is rejected. Startup checks PostgreSQL 18, takes schema-scoped exclusive writer ownership, and migrates before listening. One process per schema is supported. A second server/importer/seed command is refused. Connection loss never reacquires ownership for stale in-memory game state; restart the app after restoring the database.

The pool permits eight connections, two idle, with a five-minute idle lifetime. A dedicated connection holds game ownership and executes game mutations. Connection/transaction deadlines default to five seconds. Catalog transactions take their own schema advisory lock. Public aggregates use read-only Repeatable Read snapshots. PostgreSQL durable defaults remain enabled.

`GET /api/health` probes current pool/schema/writer readiness with a one-second deadline. It returns 200 with `status: "ok"`, `database: "postgresql"`, `ready: true`, `schema_version: 4`; failure returns 503, `status: "unavailable"`, `ready: false`. There is no `wal` field. Configuration and API errors do not echo connection strings, passwords, or row values.

`make build` always builds the frontend first. The committed `web/dist/.gitkeep` allows Go builds on a clean checkout. Node is not required to run the resulting app. `make dev-web` alone needs the matching Go account API; public discovery needs the matching Go API, and catalog management needs authorized API access.

The shared `internal/testpg` fixture requires `TEST_DATABASE_URL`, generates a unique `test_*` schema, pins the schema on every connection, and drops only that owned schema after closing stores. Missing PostgreSQL fails required tests; no SQLite fallback or silent skip exists. The load harness similarly creates a random `test_load_*` schema through `scripts/dbfixture`, owns its server child, and drops that schema on exit. It ignores operational `DATABASE_SCHEMA`/`DATABASE_URL` in favor of explicit test configuration.

PostgreSQL migration versions are independent of SQLite source version 4. The ledger and DDL commit together; newer ledgers are refused. Migration does not reinterpret old game balances or JSON.

Migration 2 (`002_accounts_and_watchlists.sql`) adds `commerce_accounts`, digest-only `commerce_sessions`, owner/lot-keyed `commerce_watchlist`, and `commerce_account_seed_runs`. Migration 1 stays immutable. Existing game identities, amounts, revisions, catalog publication, and the admin capability stay unchanged; no accounts are inferred from players. Health reports the supported ledger version (currently 4). Migration and ledger changes are atomic, including upgrades from populated version 1. Test targets above require an explicit disposable PostgreSQL database.

Before upgrading, stop the sole writer and back up the application schema with your PostgreSQL backup tooling. Restart a schema-4-compatible build to migrate. For rollback, keep the service stopped and use a compatible build or explicitly restore the pre-upgrade backup; restoring that snapshot loses all later changes, including bids/results and account/watchlist changes. Schema-2 binaries refuse schema 3; never lower the ledger to run an older binary. There is no automatic table deletion or down-migration. A first offline SQLite import refuses any target containing commerce rows, including account seed markers, auction/bid/result rows and orders/simulated receipts; it never converts legacy players into accounts.

## Explicit demo account provisioning

Stop the sole schema writer, set DATABASE_URL/DATABASE_SCHEMA privately, and provide `DEMO_BIDDER_ONE_PASSWORD`, `DEMO_BIDDER_TWO_PASSWORD`, and `DEMO_ADMIN_PASSWORD` through your operator environment/secret manager. Each must satisfy the documented 12–128-character password policy. Then run:

```sh
./bin/auctionlab --seed-accounts
```

This takes the writer lease, migrates and exits before game, listener or tunnel creation. It cannot be combined with import, verify-only, seed-catalog, seed-at or tunnel flags. First execution returns only `{"status":"seeded"}`; repeats return `{"status":"already_applied"}` without requiring password inputs. Normal startup never reads these three variables or creates accounts. Restart the server after maintenance.

Seed version 1 atomically creates labeled demo bidders `bidder-one@demo.auctionlab.test` and `bidder-two@demo.auctionlab.test`, plus demo admin `operator@demo.auctionlab.test`. A preexisting reserved email without the completion marker aborts the whole seed. Repeats preserve IDs, passwords, sessions and memberships. No default password exists and no supplied password is printed. Account roles are independent of the catalog capability; the admin account alone cannot manage catalog content. Catalog seeding remains separate and does not create accounts. A working development frontend needs the Go account API through its same-origin /api proxy.

## Timed auction upgrade (schema 3)

Migration 3 (`003_timed_auctions.sql`) adds independent `commerce_lot_auctions`, immutable accepted `commerce_bids`, and once-only `commerce_auction_results`. Migrations 1–2 are unchanged. The ledger and new DDL commit atomically. Migration alone neither executes auctions nor accepts bids. The current build also applies migration 4 and health requires schema 4. Stop the sole writer and back up the schema before upgrading; an old schema-2 binary cannot operate on schema 3. Rollback requires a schema-3-compatible binary or explicit restoration of the stopped pre-upgrade backup, losing subsequent bids/results; do not delete auction tables, lower the ledger, or copy data back into SQLite.

Normal startup adopts all previously published original schedules before listening. Future lots wait; eligible lots open; expired lots without accepted bids become unsold. Maintenance/import/seed commands do not run this lifecycle. Prepare a new future-scheduled draft event to rehearse again: published dates and results are immutable. Auctions and account mutations share the existing leased writer connection; ownership loss requires restart and never reacquires the lease. Reads use the pool and hold no writer lease.

## Winner checkout upgrade (schema 4)

Migration 4 (`004_winner_checkout.sql`) adds `commerce_orders` and `commerce_demo_payments`, sold-result/owner composite references, unique order/payment/request keys, integer/state checks, and an unpaid-owner index. Migrations 1–3 remain unchanged. Fresh initialization and upgrades preserve game amounts/revisions, accounts/sessions/watchlists, publication/schedules, accepted bids and results. The ledger and all migration DDL commit together; a failed migration leaves schema 3 intact. Migration alone creates no orders or payments.

Normal startup subsequently drains missing sold-result orders before HTTP readiness; background reconciliation uses the same bounded pass. New sold closure commits result/order atomically. For historical adoption, `closes_at` and `recorded_at` preserve the original result, while `created_at` is the adoption time. Existing order IDs/snapshots/paid selections/receipts are never rewritten by reconciliation. Offline import and account/catalog seed modes return before adoption. The SQLite importer treats both purchase tables as occupied; it never merges purchase data or changes legacy units.

Before upgrading, stop the sole writer and take a recoverable schema-3 backup. Start the schema-4 build and verify `/api/health` reports version 4 and readiness. Older binaries refuse schema 4. Prefer roll-forward fixes after purchases exist. A restore requires the writer stopped and an explicit decision accepting loss of post-backup bids, accounts and simulated purchase data; never lower the ledger or delete purchase tables to run an old binary. No destructive down migration or automatic reset is provided. Checkout and closure use the leased writer, bounded deadlines and commit-before-success; lost leases fail readiness and are never reacquired by stale memory. Unknown confirmation commits require authenticated durable readback.

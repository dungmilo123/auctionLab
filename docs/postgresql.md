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

`GET /api/health` probes current pool/schema/writer readiness with a one-second deadline. It returns 200 with `status: "ok"`, `database: "postgresql"`, `ready: true`, `schema_version: 2`; failure returns 503, `status: "unavailable"`, `ready: false`. There is no `wal` field. Configuration and API errors do not echo connection strings, passwords, or row values.

`make build` always builds the frontend first. The committed `web/dist/.gitkeep` allows Go builds on a clean checkout. Node is not required to run the resulting app. `make dev-web` alone needs the matching Go account API; public discovery needs the matching Go API, and catalog management needs authorized API access.

The shared `internal/testpg` fixture requires `TEST_DATABASE_URL`, generates a unique `test_*` schema, pins the schema on every connection, and drops only that owned schema after closing stores. Missing PostgreSQL fails required tests; no SQLite fallback or silent skip exists. The load harness similarly creates a random `test_load_*` schema through `scripts/dbfixture`, owns its server child, and drops that schema on exit. It ignores operational `DATABASE_SCHEMA`/`DATABASE_URL` in favor of explicit test configuration.

PostgreSQL migration versions are independent of SQLite source version 4. The ledger and DDL commit together; newer ledgers are refused. Migration does not reinterpret old game balances or JSON.

Migration 2 (`002_accounts_and_watchlists.sql`) adds `commerce_accounts`, digest-only `commerce_sessions`, owner/lot-keyed `commerce_watchlist`, and `commerce_account_seed_runs`. Migration 1 stays immutable. Existing game identities, amounts, revisions, catalog publication, and the admin capability stay unchanged; no accounts are inferred from players. Health reports the supported ledger version (currently 2). Migration and ledger changes are atomic, including upgrades from populated version 1. Test targets above require an explicit disposable PostgreSQL database.

Before upgrading, stop the sole writer and back up the application schema with your PostgreSQL backup tooling. Restart a schema-2-compatible build to migrate. For rollback, keep the service stopped and use a compatible build or explicitly restore the pre-upgrade backup; restoring that snapshot loses later account/watchlist changes. Older version-1 builds refuse version 2. There is no automatic table deletion or down-migration. A first offline SQLite import refuses any target containing commerce rows, including account seed markers; it never converts legacy players into accounts.

## Explicit demo account provisioning

Stop the sole schema writer, set DATABASE_URL/DATABASE_SCHEMA privately, and provide `DEMO_BIDDER_ONE_PASSWORD`, `DEMO_BIDDER_TWO_PASSWORD`, and `DEMO_ADMIN_PASSWORD` through your operator environment/secret manager. Each must satisfy the documented 12–128-character password policy. Then run:

```sh
./bin/auctionlab --seed-accounts
```

This takes the writer lease, migrates and exits before game, listener or tunnel creation. It cannot be combined with import, verify-only, seed-catalog, seed-at or tunnel flags. First execution returns only `{"status":"seeded"}`; repeats return `{"status":"already_applied"}` without requiring password inputs. Normal startup never reads these three variables or creates accounts. Restart the server after maintenance.

Seed version 1 atomically creates labeled demo bidders `bidder-one@demo.auctionlab.test` and `bidder-two@demo.auctionlab.test`, plus demo admin `operator@demo.auctionlab.test`. A preexisting reserved email without the completion marker aborts the whole seed. Repeats preserve IDs, passwords, sessions and memberships. No default password exists and no supplied password is printed. Account roles are independent of the catalog capability; the admin account alone cannot manage catalog content. Catalog seeding remains separate and does not create accounts. A working development frontend needs the Go account API through its same-origin /api proxy.

# Proposal

## Why

Change 1 provides the vintage website shell, but catalog administration still shows previews and all retained persistence uses SQLite. Change 2 will deliver durable catalog management on PostgreSQL and an explicit migration of existing SQLite data before later discovery and bidding features build on that storage.

## What Changes

- **BREAKING:** Replace SQLite as the application's runtime database with PostgreSQL 18, accessed from Go through `database/sql` and `pgx/v5/stdlib`. Move retained game/configuration storage as well as the new catalog; require `DATABASE_URL`, reject obsolete runtime `DB_PATH`, and provide no automatic SQLite fallback.
- Add PostgreSQL schema migrations and an explicit, offline, read-only-source SQLite-v4 import/verification mode. Preserve existing IDs, tokens, admin capability, amounts, JSON text, revisions, timestamps, sealed bids, and generated-ID high-water marks; validate before cutover and retain the source for rollback.
- Replace SQLite-only WAL health/configuration, SQL dialect assumptions, connection serialization, and temporary-file test fixtures with PostgreSQL readiness, bounded pooling, explicit transaction coordination, and isolated real-PostgreSQL tests.
- Introduce a commerce catalog independent of legacy game items: five vintage categories; lots with ordered local images, description, brand, era, condition, specifications, supplied story/provenance, and delivery/pickup information; events with theme, cover, description, schedule, and ordered lots.
- Replace `/admin/lots` and `/admin/auctions` previews with authorized lists and draft editors. Operators can save incomplete drafts, assign/reorder lots, inspect the calculated schedule and public projection, and publish a complete event atomically.
- Confirm the roadmap defaults: overlapping events; one shared opening time per event; staggered lot closes; manual ascending auctions with fixed per-lot increments. Persist configuration only; opening, bidding, closing, and settlement remain Change 5.
- Define new monetary values as integer USD cents (`currency: "USD"`, explicit `*_minor` fields), independent of legacy whole-unit game amounts. Provide exact decimal entry/display in admin without reusing the game formatter.
- Publish an event and its lots as one validated aggregate. Keep published records immutable in this change; draft editing and reassignment remain available before publication. No deletion, unpublish, or relisting workflow is introduced.
- Add explicit public catalog projections and read APIs for Change 3, excluding drafts and admin notes. Keep storefront/account/order previews honest until their corresponding changes.
- Add an explicit, repeat-safe demo seed for 18 lots across three themed events, with bundled local illustrations and fixed persisted schedules. Normal startup never seeds, overwrites content, or moves dates.
- Extend the existing admin capability to every catalog admin API, with JSON authorization errors and mutation origin checks; retain the document gate. Account-based roles remain Change 4.

Out of scope: discovery page implementation, accounts/watchlists, image uploads or remote media fetching, category management UI, new live bidding/winner behavior, orders/payments, analytics, zero-downtime or bidirectional database replication, and conversion of legacy game balances into commerce money. Existing game behavior is ported and regression-tested, not removed.

## Capabilities

### New Capabilities

- `postgresql-persistence`: PostgreSQL-only runtime, versioned schema, explicit verified SQLite import, preserved legacy semantics, health/readiness, and cutover/rollback behavior.
- `vintage-catalog`: Durable category and lot records, commerce money/media contracts, public projections, and isolated demo content.
- `auction-event-publishing`: Draft events, lot assignment/order, deterministic schedules, validation, atomic publication, and published-record immutability.
- `catalog-administration`: Capability-protected catalog APIs and accessible bilingual admin preparation/editing/publishing screens.

### Modified Capabilities

- `website-navigation`: Replace only catalog admin previews with working management pages and narrow the preview/no-mutation rule accordingly; preserve the route set, other preview boundaries, and retired game routes.

## Impact

- Port all runtime repositories in `internal/store/` to PostgreSQL, with a new PostgreSQL migration ledger independent of SQLite's `user_version`. Add catalog tables/repositories and a separate catalog service. Keep `modernc.org/sqlite` only in the isolated source reader for the import command.
- Update server configuration/boot/health, `go.mod`/`go.sum`, Makefile targets, database-backed game/hub/server tests, and load/tunnel harnesses. Add PostgreSQL provisioning/test instructions and reconcile SQLite-only claims in active project documentation during implementation.
- Add `/api/admin/catalog/*` and `/api/catalog/*` handlers in `internal/api/` and wire them in `cmd/server/main.go`. Reuse the existing capability cookie and comparison; never authorize catalog APIs through URL keys or player tokens.
- Update `web/src/routes/admin-preview/AdminPage.tsx`, `App.tsx`, shared form/layout components, and EN/VI website copy; add a typed catalog HTTP client and commerce money/schedule helpers. Preserve native document navigation and existing public preview fixtures.
- Bundle licensed/project-authored seed media, add an opt-in seed command and operator documentation, and cover migration, restart, publication, authorization, and public-field exclusion with targeted tests and browser acceptance.
- Keep Go/React, no-CGO builds, and frontend-first embedding into one application binary. PostgreSQL becomes a required separate service; a local instance supports offline demos. This explicitly replaces the former SQLite/no-external-DB constraint; PostgreSQL is not bundled into the binary. Planning artifacts remain local under the existing OpenSpec ignore rule.

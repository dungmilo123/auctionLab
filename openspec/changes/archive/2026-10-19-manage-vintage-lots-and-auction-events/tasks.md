# Tasks

This checklist was created during planning. Checked tasks record implementation and verification performed during apply; see `verification/REPORT.md` for measured evidence and limits.

## 1. Replace runtime SQLite with PostgreSQL

- [x] 1.1 Add an exact compatible pgx v5 dependency and PostgreSQL connection configuration using `DATABASE_URL`/validated `DATABASE_SCHEMA`, bounded `database/sql` pooling, deadlines, and credential-safe errors; verify required/invalid/unreachable configurations fail before listening and runtime `DB_PATH` cannot select SQLite.
- [x] 1.2 Replace SQLite DDL/pragma versioning with ordered PostgreSQL migrations and a ledger starting at version 1, covering eight legacy tables, six catalog tables, import metadata, identities, constraints, and reference categories; verify fresh/repeated migration, transactional failure, and newer-ledger refusal on PostgreSQL 18.
- [x] 1.3 Port legacy repository SQL to numbered parameters, BIGINT/null-preserving fields, text-preserving legacy JSON, identity/RETURNING behavior, deterministic text collation/order, and PostgreSQL SQLSTATE errors; verify snapshot, bid, sealed-bid, config/key, duplicate-nickname, and reset behavior against PostgreSQL.
- [x] 1.4 Add schema-scoped session writer ownership on a dedicated connection used by game mutations and maintenance writes; verify a competing process is refused and connection loss stops game writes without reacquiring a lease for stale memory or publishing an uncommitted update.
- [x] 1.5 Replace WAL health output with bounded current PostgreSQL/schema/ownership readiness; verify HTTP 200/503, absence of `wal`/credentials, and failure responses during a simulated DB outage without fabricated successful saves.
- [x] 1.6 Add a shared `TEST_DATABASE_URL` helper with unique disposable schemas, per-connection schema configuration, lease cleanup, and multi-connection support; verify parallel fixtures cannot see/drop each other's data and missing required test service fails clearly.
- [x] 1.7 Port store fixtures/tests to real PostgreSQL and replace pragma/SQLite-only acceptance with ledger, constraint, pool, transaction, failure, and reconnect tests; verify required store checks run without SQLite runtime fallbacks or silent skips.
- [x] 1.8 Port database-backed game/hub/server fixtures including crash/restart, commit-before-publish, revision ordering, atomic budget/settlement, secret projection, and tunnel tests; verify those existing behavioral regressions pass against isolated PostgreSQL schemas.
- [x] 1.9 Update Makefile development/run/test/load targets and `scripts/loadtest-acceptance.sh` to explicit PostgreSQL configuration and disposable schema ownership; verify 20-client ordering/resync and tunnel harnesses cannot target or clean unrelated databases/schemas.
- [x] 1.10 Document local PostgreSQL 18 provisioning/offline operation, app/test roles, configuration, health changes, separate-service requirement, and unchanged no-CGO frontend-first build; reconcile active ROADMAP/PROJECT/PLAN/AGENTS/README SQLite claims and verify documented commands against a disposable instance while retaining archived history.

## 2. Implement explicit SQLite import and cutover verification

- [x] 2.1 Add an isolated read-only SQLite-v4 source reader under `internal/sqliteimport/`; verify version/columns, integrity/references, typed values/nulls, and consistent-snapshot requirements while source database/sidecar bytes remain unchanged, and reject incompatible sources rather than rewriting them.
- [x] 2.2 Implement all-eight-table import into an initialized unused PostgreSQL schema with explicit IDs, raw legacy JSON/text, tokens/key, amounts, ordering, timestamps/revisions, and runtime state; verify a populated representative source matches target typed rows without game bootstrap, settlement, key minting, or currency conversion.
- [x] 2.3 Preserve bid/event identity high-water marks from imported maxima and SQLite sequence history using transactional sequence RESTART; verify empty tables and previously deleted high IDs, new generated IDs, overflow rejection, and rollback on injected failure.
- [x] 2.4 Add canonical typed row counts/digests, reference checks, and a completion marker committed with the import; verify failed imports leave no partial data/marker, repeats do not overwrite edits, and occupied targets/different-source imports are refused.
- [x] 2.5 Add independent verify-only comparison of every source/target row and identity readiness; verify changed values/missing rows fail even with a prior marker, output excludes credentials/row secrets, and neither database, sequence, nor migration metadata is written.
- [x] 2.6 Wire mutually exclusive `--import-sqlite <snapshot>`, `--verify-only`, and seed modes into the binary without starting engine/listener/tunnel; verify flag errors, read-only verification, source-only use of modernc, and startup remains PostgreSQL-only.
- [x] 2.7 Document stopped-writer snapshot/WAL preparation, supported v4 source, media inventory, unused-target import-before-start ordering, verification, PostgreSQL backup/restore, cutover, and pre-/post-write rollback limits; verify the runbook in a disposable rehearsal and preserve the source plus prior binary without exposing secrets.

## 3. Add independent catalog repositories on PostgreSQL

- [x] 3.1 Add catalog domain/repository interfaces and PostgreSQL draft/category/lot/event reads/writes, ordered images/specifications, nullable fields, versions, and membership storage; verify real PostgreSQL round trips and application reconnect preserve values/order with no dependency on game items or truth values.
- [x] 3.2 Add catalog/game isolation checks around the ported snapshot/reset paths; verify imported legacy game reset preserves catalog tables and catalog writes do not change game state/revision or the persisted capability.
- [x] 3.3 Document the catalog's PostgreSQL table/field contract and boundary from legacy units in the active README; verify examples match migrations and tested round trips, with no SQLite-v5 schema or file-rollback claim remaining.

## 4. Define draft validation, money, schedules, and media

- [x] 4.1 Implement stable ID/slug generation, duplicate handling, title-only draft creation, category/enum checks, and optional historical fields; verify draft validation tests preserve incomplete content, reject invalid supplied values, and never regenerate slugs on title edits.
- [x] 4.2 Implement server-side integer USD-cent and fulfillment validation with the documented maximum; verify zero opening price/fee, positive increments, missing publication terms, fractional/negative/over-limit money, and alternate currencies are handled as specified.
- [x] 4.3 Implement shared server schedule derivation with an injected clock and checked arithmetic; verify common opening, staggered closes, reorder recalculation, overlapping events, invalid/overflowing dates, and expired-opening publication rejection.
- [x] 4.4 Implement canonical local-media validation and an injected filesystem resolver for runtime and embedded media; verify allowed images load, missing publication files fail, and external/encoded traversal/symlink escapes are rejected without remote fetching.
- [x] 4.5 Document catalog field, money, timezone, draft-completeness, and public media-storage rules in `web/README.md`; verify its examples match the tested request fields and the `IMAGES_DIR`/Makefile behavior.

## 5. Implement membership, publication, and public projections

- [x] 5.1 Implement atomic draft lot assignment/removal/reorder under the schema-scoped catalog transaction advisory lock using full ordered membership replacement; verify duplicate, missing, already-assigned, and published lots cannot partly change membership and draft detach/reattach works.
- [x] 5.2 Add expected-version updates, parent-event version advancement on member edits, and published-record write guards; verify stale saves conflict and published content/prices/slugs/timing/membership cannot be modified.
- [x] 5.3 Implement aggregate publication validation and one-transaction event/lot transition with catalog locking and expected versions; verify complete publication, field/lot-specific failure, storage rollback, repeat publication, and edit-versus-publish concurrency using real PostgreSQL across multiple connections.
- [x] 5.4 Implement explicit public event/lot DTOs and authorized draft preview using consistent PostgreSQL read snapshots and the same projection/schedule mapper; verify draft/nonexistent slugs are indistinguishable publicly and private-note/key/game-secret sentinels never appear in published JSON.
- [x] 5.5 Verify the stored published aggregate after application/PostgreSQL restart and after the injected clock passes planned close; confirm content/order/times remain unchanged and no bid, winner, order, game revision, or live/ended status is created.
- [x] 5.6 Document event-owned publication, immutable published records, derived planned dates, and the lack of realtime execution in `web/README.md`; verify examples align with projection output and publication tests.

## 6. Expose protected admin and public catalog APIs

- [x] 6.1 Add a JSON-specific admin guard reusing the capability cookie/comparison, covering every private read/write/preview and private namespace fallback; verify missing/wrong cookie, query-key-only, and player-token requests yield JSON 401 with no key, data, gate HTML, or redirect.
- [x] 6.2 Add JSON/custom-header mutation checks and effective-origin validation with narrowly trusted loopback proxy scheme handling; verify direct LAN, Vite preserved-Host, and TLS-terminated requests succeed while foreign/null origins, untrusted forwarded headers, form requests, and missing headers fail without writes.
- [x] 6.3 Add and wire admin list/create/detail/update/membership/preview/publish endpoints in `internal/api/` and `cmd/server/main.go`; verify real HTTP requests return the designed success/error envelopes, status codes, no-store policy, bounded decoding, and field errors.
- [x] 6.4 Add public categories/event/lot list and slug-detail endpoints with consistent aggregate reads; verify anonymous published-only visibility, draft/missing 404 behavior, private-field exclusion, and unchanged unknown-API/document-gate behavior.
- [x] 6.5 Document the endpoint table, expected versions, mutation header, error codes, and public-field contract in `web/README.md`; verify representative documented create/edit/preview/publish requests against an isolated test server without recording credentials.

## 7. Add explicit repeat-safe demo seeding

- [x] 7.1 Prepare 18 distinct illustrative lots across the three named events, covering all five categories, complete content/prices/fulfillment, and local galleries/covers; verify dataset validation passes and bundle project-authored/licensed media with source information in the asset documentation.
- [x] 7.2 Implement one-transaction seed version 1 and its marker with stable identities and reference-derived schedules; verify 18 lots/three draft events, repeat-after-edit preservation, fixed dates, collision rejection, and rollback on failure.
- [x] 7.3 Add `--seed-catalog --seed-at <RFC3339>` seed-only execution using PostgreSQL configuration and exclusive writer ownership; verify it exits without a listener/engine/tunnel, rejects invalid flag combinations, and ordinary startup neither seeds nor rebases content.
- [x] 7.4 Document build/seed/replay commands, draft review/publication, expired-demo handling, and asset labels in `web/README.md`; verify the built binary can seed a disposable PostgreSQL schema and a normal authorized publication validates its bundled media.

## 8. Add the frontend catalog contract and preparation controls

- [x] 8.1 Add a typed catalog HTTP client with cookie credentials, mutation headers, envelopes, stable error mapping, and authorization/backend-failure states; verify mocked request/response tests cover 401, 409, 422, and transport failures without embedding or storing keys.
- [x] 8.2 Add independent exact commerce decimal parse/format helpers and UTC/local schedule input helpers; verify cents round trips, maximum/invalid inputs, timezone-preserving conversion, and missing schedule fields while leaving legacy `formatUSD` tests unchanged.
- [x] 8.3 Add query-based new/edit selection within existing routes and route-specific management notices; verify direct links/unknown selections and existing route normalization tests, while public/account/order notices and retired `/screen` routes keep their behavior.
- [x] 8.4 Extend reusable labeled textarea/select/ordered-list controls and EN/VI management/error copy; verify copy completeness, accessible label/help/error associations, keyboard order actions, and language switching without translating or clearing entered content.
- [x] 8.5 Add access-required/backend-unavailable recovery and same-hostname Go-gate instructions for Vite; verify no private requests succeed without the cookie and document the development flow in `web/README.md` without a frontend secret field or bypass.

## 9. Build working lot and event admin pages

- [x] 9.1 Replace the lot-management preview with a stored list, title-only creation, and query-selected draft editor for identity/category/content/notes; verify creating, saving, refreshing, and reopening retains the entered record and shows assignment/publication state.
- [x] 9.2 Add ordered gallery/alt-text and specification editing, exact money input, and disclosed delivery/pickup fields to the lot editor; verify ordering and cents persist, invalid saves retain input, and published lot detail is read-only.
- [x] 9.3 Replace the event-management preview with a stored list and draft editor for content/cover, available-lot selection, detach/reorder, and schedule input; verify direct refresh and accessible move controls retain membership/order and display server-derived planned times.
- [x] 9.4 Add authorized public-field preview, explicit Publish, per-lot validation links, committed success, conflict/reload handling, and read-only published event detail; verify incomplete publication exposes nothing and a corrected event publishes all members with no duplicate records.
- [x] 9.5 Exercise meaningful form outcomes while building both pages: loading/empty/not-found, backend failure, lost authorization, validation, stale save, and pending submission; verify no false success, no input loss on failed saves, no automatic overwrite of a newer record, and committed-state reload after an uncertain database commit.
- [x] 9.6 Update the README's route/component/feature descriptions and admin overview links for working catalog management; verify public discovery/account/order pages still use honest previews and no legacy game view or socket is mounted.

## 10. Verify migration, operations, and catalog integration

- [x] 10.1 Run `make test`, `make test-race`, and `make build` with explicit isolated PostgreSQL test configuration after final edits; record actual outcomes and confirm the frontend-first `CGO_ENABLED=0` binary build, local assets, existing game regressions, and placeholder behavior still pass.
- [x] 10.2 With the built binary and an isolated PostgreSQL acceptance schema, seed, authorize, edit a lot, create/edit an event, assign/reorder lots, inspect preview, correct a publication error, publish, query anonymous public JSON, and restart; save acceptance evidence proving durable values/order/dates, published immutability, repeat publication, and private-field exclusion.
- [x] 10.3 Inspect both working admin pages in EN/VI at 320/375/768/1280 widths, keyboard-only, 200 percent text zoom, and missing-image/offline conditions; save representative screenshots and concise results under this change's `verification/`, distinguishing automated checks from visual observations.
- [x] 10.4 Verify built Go authorization and same-hostname Vite authorization with missing/wrong/valid cookies, direct editor URLs, and mutation origin rejection; confirm frontend-only access cannot view drafts or simulate saves and capture results without credentials.
- [x] 10.5 Recheck public/account/order preview boundaries, normal startup without seed, retired game routes, and absence of catalog lifecycle execution after planned times; map evidence to the five delta specs and run strict OpenSpec validation before handing off.
- [x] 10.6 Rehearse stopped-writer SQLite-v4 import, independent verify-only, first PostgreSQL boot, persisted-key access, legacy deadline resume, and a rejected competing writer in disposable targets; record typed equality before boot and distinguish expected legacy settlement after boot from migration drift.
- [x] 10.7 Exercise PostgreSQL outage/readiness/restart, backup/restore into a disposable schema, and rollback to the prior binary plus untouched SQLite snapshot; verify no source modification, no automatic reverse sync, and clear disclosure of post-cutover writes in the credential-free acceptance report.

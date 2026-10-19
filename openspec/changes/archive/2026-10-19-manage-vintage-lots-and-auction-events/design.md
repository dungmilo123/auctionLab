# Design

## Context

See `proposal.md` for motivation and the five delta specs for behavior. Read-only inspection found:

- `web/src/App.tsx` uses the explicit matcher in `lib/route.ts` and native document links. `/admin/lots` and `/admin/auctions` mount `routes/admin-preview/AdminPage.tsx`, which only renders unavailable states. Public pages use `fixtures/site.ts` and generic detail slugs.
- `layouts/SiteLayout.tsx` mounts the same `PreviewNotice` on every page. `components/site/Presentation.tsx` and `lib/site-copy.ts` supply reusable controls, image fallback, and typed EN/VI copy. Management needs truthful, route-specific notices rather than removing public notices.
- `internal/api/admin_gate.go` gates admin **documents** using `auction_admin`, a constant-time comparison, and a fixed `/admin` redirect. It returns HTML when unauthorized, so wrapping JSON handlers with `AdminGate` would be incorrect. The API currently has health and a JSON fallback, not catalog CRUD.
- `internal/store/schema.go` owns DDL and migrations, currently `UserVersion = 4`. `store.Open` already sets per-connection WAL/foreign-key pragmas and a one-connection pool. Existing tables represent players, a single-current-item game, bids, runtime, and server configuration; they do not model concurrent catalog events.
- Game writes/reset in `internal/store/snapshot.go` address their existing tables explicitly. Keep that boundary, and verify catalog survival across game reset. `cmd/server/main.go` loads the old engine and WS hub; catalog publication must not invoke them.
- `/images/` already serves operator-managed local files, without directory listings, from `IMAGES_DIR` (`images` by default; Makefile currently supplies `./image`). `/site-assets/` is embedded public media with real missing-file responses. Neither namespace is private storage.
- `web/src/lib/format.ts::formatUSD` treats an integer as whole game units and emits `.00`; it cannot format commerce cents. Current game configuration saying USD does not define commerce units.
- `web/vite.config.ts` proxies `/api` without changing the browser-facing Host. Vite admin documents currently bypass the Go gate because they contain no data. This change must enforce data authorization even when the frontend document is public.
- Main specs currently require admin previews and forbid catalog mutations. The `website-navigation` delta updates those requirements explicitly; the design-system contract remains applicable unchanged. `openspec/` is generally ignored, with the previous archived change explicitly tracked; this proposal does not change ignore rules.

## Goals / Non-Goals

**Goals:** move all runtime persistence to PostgreSQL with a verifiable SQLite cutover; preserve legacy game correctness; introduce a separate transactional catalog domain, safe public DTOs, working admin preparation, and repeat-safe demo content.

**Non-Goals:** no game-rule redesign, catalog auction execution, dual runtime database support, zero-downtime replication, reverse PostgreSQL-to-SQLite synchronization, upload service, full accounts, category CRUD, or published-content amendment workflow. Public discovery wiring remains Change 3. The user-requested PostgreSQL migration supersedes the SQLite/no-external-database constraints in existing project documents; those documents describe the current implementation until apply updates them.

## Decisions

### 1. PostgreSQL for the whole application, with an isolated catalog domain

Use PostgreSQL **18.x** as the supported/tested server major and `github.com/jackc/pgx/v5/stdlib` through the existing `database/sql` abstraction. Pin an exact compatible pgx v5 release in `go.mod`/`go.sum` during apply. The [official stdlib adapter](https://github.com/jackc/pgx/blob/master/stdlib/sql.go) supports this approach; use PostgreSQL `$1`, `$2` parameters and `pgconn.PgError` SQLSTATE classification. Do not introduce a second `pgxpool` beneath `database/sql`.

Require `DATABASE_URL`; add `DATABASE_SCHEMA` defaulting to `auctionlab`, validated as a simple identifier and quoted consistently. Configure the selected schema on **every** connection, not with a one-off pooled `SET search_path`; qualify migration/ownership queries and disallow untrusted schema search paths. Reject `DB_PATH` in normal server/seed modes with guidance to the importer. Missing configuration or a bounded connection/schema probe fails startup before the listener. Never log raw URLs, parser errors containing credentials, or database error detail containing row values. Use explicit local loopback credentials/TLS settings in operator setup; remote connection security follows the supplied connection configuration without silently disabling TLS.

The default pool has eight open connections and two idle, with one connection reserved for game-writer ownership, a five-minute idle lifetime, and five-second connection/startup probe deadlines. Use bounded request/transaction contexts and a one-second health probe. These are small-demo defaults, not scaling claims. Retain PostgreSQL's durable defaults; do not disable synchronous commit or server durability settings. PostgreSQL is a separately provisioned local or remote service. The Go binary still embeds the frontend and remains `CGO_ENABLED=0`; it no longer contains its operational database. Offline demos require a local PostgreSQL process to remain reachable.

Replace `PRAGMA user_version` with ordered embedded PostgreSQL migrations and a `schema_migrations` ledger, starting at PostgreSQL version **1**. This version is independent of SQLite source version 4. The initial transaction creates the eight retained game/configuration tables, the six catalog tables below, an import completion table, reference categories, and the ledger entry. Migration runs under the schema's exclusive writer ownership; each migration step and its ledger entry commit together. Fail on a newer ledger and on failed DDL. Do not rewrite or increment any source SQLite file's version. Normal startup bootstraps a missing game row/admin capability only **after** migrations; import mode must execute before that bootstrap against an unused target.

| Table | Stored content / constraints |
|---|---|
| `catalog_categories` | Stable category ID/slug, EN/VI display names, sort order; five built-in rows |
| `catalog_lots` | ID, unique slug, title, nullable category, description, optional brand/era/story, condition, ordered specifications JSONB, fulfillment JSONB, nullable starting price/increment, USD currency, draft/published state, admin notes, version, timestamps, public demo flag |
| `catalog_lot_images` | Lot ID, zero-based position, local path, alt text; unique `(lot_id, position)` |
| `catalog_events` | ID, unique slug, title/theme/description, cover path/alt, nullable planned times, draft/published state, admin notes, version, timestamps, public demo flag |
| `catalog_event_lots` | Event ID, lot ID, zero-based position; unique lot ID and unique `(event_id, position)` |
| `catalog_seed_runs` | Seed version and persisted reference time, written only after complete seed success |

Use `BIGINT` for money, budgets, timestamps, revisions, and generated bid/event IDs, `BOOLEAN` for new flags, and foreign keys/CHECK constraints for enums, USD/bounds, and positions. Preserve legacy JSON-bearing columns as **TEXT** so import preserves exact text; new structured catalog fields use JSONB arrays/objects without changing order within arrays. Nullable draft fields distinguish absent from valid zero values. Preserve application-computed nickname keys and deterministic text ordering with explicit `COLLATE "C"` for legacy keys/tie-breaks; do not substitute locale/case-insensitive database rules. Use `GENERATED BY DEFAULT AS IDENTITY` for legacy append-only IDs and `INSERT ... RETURNING` when a generated ID is needed, never `LastInsertId`. Handle uniqueness with SQLSTATE `23505`, and classify foreign-key/check violations separately rather than treating every PostgreSQL error as a duplicate nickname.

Port **all** runtime SQL in `schema.go`, `snapshot.go`, `items.go`, and `serverconfig.go`, including upserts, null handling, explicit ordering, and children-first reset/deletion. Preserve game state publication only after commit and the existing fatal handling of uncertain game commits. Keep `modernc.org/sqlite` only behind `internal/sqliteimport/` as a read-only migration dependency; no live store selects a backend from a file path.

Use `internal/catalog/` for types, validation, scheduling, projections, and a service accepting a narrow repository/transaction interface; implement it in `internal/store/catalog*.go`. The catalog domain does not import the concrete store or call `game.Engine`. Repositories use only their transaction handle inside a transaction and close result sets before dependent operations. PostgreSQL pooling is not a replacement for correct transaction scope.

Alternative: PostgreSQL for the catalog while keeping the game in SQLite. Rejected because it leaves two authoritative databases, two configurations, and incomplete migration. A full ORM or generic SQL compatibility layer would add scope without removing the need to verify SQL/transaction semantics.

### 2. One event owns each lot's publication and schedule

New events/lots start in `draft`. Title is required on create; validate any supplied fields immediately, but defer completeness checks until publication. Generate stable random IDs and lowercase ASCII kebab slugs; use a short ID suffix for generated collisions and an ID-based fallback if a title has no usable ASCII characters. Explicit duplicate slugs produce a conflict. Renaming a title does not silently change the slug. Submitted slugs are lowercase ASCII letters/digits separated by hyphens, maximum 96 characters.

There is no standalone lot-publish action: an unassigned draft or a lot in a draft event remains private. Publish changes the event and all its members together. All content, slug, money, fulfillment, membership, and planned dates are read-only once published. A lot cannot be in two events; moving one between drafts requires detach then attach. No deletion, cancellation, unpublish, or relisting endpoint is added.

Use full ordered lot-ID replacement for event membership. Validate the complete submitted list before changing rows; replace order atomically to avoid transient uniqueness collisions. Member writes advance affected lot versions and the event version. Editing an assigned draft lot advances both its own version and its parent event version in the same transaction.

Coordinate every catalog mutation with a PostgreSQL transaction-scoped advisory lock keyed by the selected database/schema, acquired **before** loading records. Use Read Committed for these short serialized preparation transactions, then expected-version guards and constraints. This intentionally serializes catalog writes at demo scale while allowing pooled readers and separate game writes. It covers unassigned-lot attachment races without a fragile event/lot lock order or an in-process-only mutex. The lock is distinct from the session writer lease. Publication, membership, draft edits, and seed transactions all follow the same protocol. PostgreSQL releases transaction locks at transaction end; see [explicit locking](https://www.postgresql.org/docs/18/explicit-locking.html).

Each edit submits the loaded `version`; update using a compare-and-set guard and return 409 for stale versions. Membership replacement and event publication submit the event version. Publication reloads members and validates within the transaction, then transitions only current draft rows. A concurrent successful lot edit invalidates an earlier event version. Repeating publish on an already-published event returns its existing result before stale-version/future-date validation; it cannot rewrite anything.

Alternative: independently published lots or editable published records. Both require additional partial-visibility or live-amendment semantics. Freezing a valid event is sufficient for the demo and gives Change 5 a stable configuration. Amendments can be specified when auction execution exists.

### 3. Store UTC schedule inputs and derive per-lot times

API/database timestamps are integer Unix milliseconds: `opens_at`, `first_closes_at`; interval is `close_interval_ms`. For lot position k, starting at zero:

`lot.opens_at = event.opens_at`

`lot.closes_at = event.first_closes_at + k * event.close_interval_ms`

The event's derived planned end is the last close. Store only event timing plus lot order; derive times through one server-side helper for admin preview and public projections, avoiding duplicated deadline columns that drift after reordering. No per-lot schedule override is supported. Events may overlap.

Validate `first_closes_at > opens_at`, positive integer interval, and all calculated instants within 0 through 8,640,000,000,000,000 milliseconds, using checked multiplication/addition. Publication additionally requires `opens_at > serverNow`; the clock is injectable for tests. Incomplete draft timing is permitted, with no fabricated zero-date display. Supplied complete timing must satisfy ordering/range rules even in drafts.

Admin date/time controls display the browser timezone explicitly, convert local input to UTC milliseconds, and show the server-derived per-lot schedule before publication. No timers, ticker registration, engine starts, result creation, or automatic status changes occur as dates pass. Publication state is editorial; never derive `live` or `ended` merely to populate existing status badges.

Alternative: independently entered lot deadlines. Rejected because the agreed shared opening/staggering model is smaller and remains consistent under reordering. Persisted deadlines and execution-state recovery belong to Change 5.

### 4. Establish cents without changing game money

New fields use `currency: "USD"`, `starting_price_minor`, `bid_increment_minor`, and `shipping_fee_minor`; the database uses integers. Supplied starting prices/fees permit zero, increments require at least one cent, and each amount is capped at 1,000,000,000,000 cents, safely below the JavaScript integer limit. Reject floats, non-finite values, alternate currencies, and over-limit values at the server boundary.

Add separate commerce parse/format helpers, e.g. `web/src/lib/catalog-money.ts`. Keep user entry as text; parse a nonnegative decimal with zero to two fractional digits by splitting/padding digit strings, then range-check the integer. Do not multiply a binary floating input and round it. Format integer cents with an explicit USD symbol/currency and two decimals. Test `0`, `0.01`, `125.50`, the maximum, excess precision, negative/overflow, and wire values with fractional cents.

Fulfillment stores enabled delivery/pickup options, delivery fee when enabled, and corresponding public instructions. Publication requires at least one option, a delivery fee and instructions for shipping, and instructions for pickup. Store specifications as ordered name/value pairs. Brand, era, and story are optional; condition/description must explain known wear without invented authenticity.

Alternative: retain whole-dollar game formatting. Rejected because cents and later checkout totals would be ambiguous. USD follows the existing site's presentation direction, but new fields explicitly define the unit and never convert legacy balances.

### 5. Local media references, explicit public/private projections

Reuse operator files at `/images/<relative-name>` and bundle seed media at `/site-assets/catalog/<file>`. No upload or remote fetch endpoint. Admin enters local paths and gallery alt text, can move images up/down, and sees image previews. Paths must be canonical application-relative paths with no `..`, backslash, query, fragment, external scheme, or protocol-relative form; reject encoded traversal/separator bypasses after URL decoding. Support JPEG, PNG, WebP, and project-authored SVG illustrations.

Inject a media resolver into the catalog service so publication verifies each referenced file against the configured images root or embedded SPA filesystem. Confine operator file lookup to its root, including symlink resolution; built-in assets are from the embedded filesystem. Drafts may reference syntactically valid files not yet installed; publication rejects missing/unreadable/non-image files and empty alt text. Later file removal falls back through the existing `LotImage` behavior; no private-media promise is made. Files in these namespaces are public even before a draft is published, so operator instructions must identify them as public media storage.

Allowlist public DTOs rather than serializing storage rows and deleting fields. Public lots include ID/slug, title/category, description/brand/era/condition, gallery/specifications/story, fulfillment, starting price/increment/currency, event identity/order, derived planned dates, editorial published state, and an honest demo-content flag. Public events include their descriptive fields, cover, derived schedule, ordered lot summaries, and demo flag. Exclude admin notes, versions, seed-run identifiers, internal timestamps, capability data, and all game values. Prices are labeled opening prices, never current bids.

Use a read-only Repeatable Read transaction (or one joined SQL statement) for an event plus its members, so a response never combines membership from different commits. Multiple SELECTs at default Read Committed are not sufficient; see [transaction isolation](https://www.postgresql.org/docs/18/transaction-iso.html). Public lot lookup requires both lot and parent event published. Draft and missing slugs return the same JSON 404. An authorized preview endpoint can project a draft through the same DTO mapper, wrapped in `{preview: true, data: ...}`; it is never exposed through public reads.

Alternative: external image URLs and arbitrary row serialization. Rejected because the demo must work offline and private-field exclusion must remain explicit as the schema grows. Seed media can reuse/extend the existing project-authored illustration style, clearly labeled as demo illustrations with sources/licenses recorded.

### 6. Add narrow HTTP APIs and authorize each admin request

Proposed contract (prefixes deliberately separate private and public data):

| Method and path | Behavior |
|---|---|
| `GET /api/catalog/categories` | Five safe reference categories |
| `GET /api/catalog/events`, `GET /api/catalog/events/{slug}` | Published list / event with ordered published lots |
| `GET /api/catalog/lots`, `GET /api/catalog/lots/{slug}` | Published list / lot detail |
| `GET /api/admin/catalog/categories` | Authorized category choices |
| `GET, POST /api/admin/catalog/lots` | List / create draft |
| `GET, PUT /api/admin/catalog/lots/{id}` | Read / replace editable lot content with expected version |
| `GET, POST /api/admin/catalog/events` | List / create draft |
| `GET, PUT /api/admin/catalog/events/{id}` | Read / replace editable event content with expected version |
| `PUT /api/admin/catalog/events/{id}/lots` | Replace ordered membership with expected event version |
| `GET /api/admin/catalog/events/{id}/preview` | Authorized public projection and schedule preview |
| `POST /api/admin/catalog/events/{id}/publish` | Validate/publish with expected event version; repeat-safe |

Use `{data: ...}` success envelopes (201 for create, 200 for other successes) and `{error: {code, fields?}}` errors. Field errors carry stable paths such as `lots.<id>.condition`, enabling EN/VI mapping without sending translated server prose. Use 400 for malformed JSON, 401 missing/invalid capability, 403 request-origin/header rejection, 404 missing resource, 409 stale/duplicate/assigned/published or rolled-back transaction conflict, 415 unsupported content, 422 valid JSON failing field/publication validation, 503 known database unavailability, and 500 other storage failures with no raw SQL or secrets. Bound JSON body size and reject unknown fields/trailing JSON. Return `Cache-Control: no-store` on catalog responses for this demo. Preserve the existing JSON API fallback for unknown routes.

Create a JSON-specific admin wrapper using only the `auction_admin` cookie and `api.Equal`; do not reuse the HTML `AdminGate` handler or accept `?k=` on APIs. All private methods, including lists/preview and private namespace misses, pass the wrapper. Mutations additionally require `application/json` and `X-AuctionLab-Admin: 1`, and reject a supplied Origin unless it equals the effective browser-facing origin. Accept a missing Origin for explicit non-browser clients only with the cookie, JSON, and custom header. Do not enable CORS credentials or wildcard origins.

Effective origin uses the request Host (including non-default port), TLS scheme, or `X-Forwarded-Proto` only from the local loopback reverse proxy used by Vite/cloudflared. Do not trust forwarded Host or scheme from arbitrary LAN clients. Reject `Origin: null`, malformed origins, and origins with credentials or non-HTTP schemes. Cover direct LAN HTTP, Vite's preserved Host, and loopback TLS termination in handler tests. A remote reverse proxy is not introduced by this change and would need an explicit trust configuration.

Alternative: rely on the document gate or issue frontend bearer keys. Both break the existing HttpOnly boundary or allow direct API access. Reusing the capability cookie with an API-specific guard provides the required authorization without bringing Change 4 forward.

### 7. Fit working editors into existing page routes

Keep the route table and native navigation. Use `/admin/lots?new=1`, `/admin/lots?edit=<id>`, `/admin/auctions?new=1`, and `/admin/auctions?edit=<id>`. Extract query selection separately from `routeFor`; IDs are passed as values to the typed API client, never inserted into HTML. List rows use real document links; save/create can update the selected URL through a normal navigation after success. A missing ID yields a record-unavailable state, not a new blank draft.

Move catalog page composition to working modules under `web/src/routes/admin/` with distinct names from retained `AdminView.tsx`. Keep `admin-preview/AdminPage.tsx` for the overview/order preview or adapt it to dispatch; do not mount the legacy WS admin view. Add typed `web/src/api/catalog.ts` request/response types, credentials on same-origin requests, stable error mapping, and the required mutation header.

Lot editor groups identity/category, descriptive content, ordered gallery, ordered specifications, money, fulfillment, and private notes. Event editor groups content/cover, available/assigned lot selection with Move up/Move down/Remove controls, timing, public preview, field readiness, and explicit Publish. Catalog list/detail display `Draft`/`Published`, not auction execution statuses. Published detail renders read-only values and explains the frozen state. Keep local form inputs on errors; a stale-version response offers reload rather than automatically overwriting newer content. Disable pending submit controls to avoid accidental repeated requests.

Reuse tokens, `Button`, `Input`, `Label`, `Field`, and `LotImage`; add labeled textarea/select/list wrappers as needed. Extend typed website EN/VI copy with field and error-code labels; stored catalog text is not translated. Avoid dragging as the only order control. Provide explicit loading, empty, backend-unreachable, access-required, not-found, validation/conflict, and server-error states.

Change the layout notice for `/admin/lots` and `/admin/auctions` to explain working content preparation and unavailable bidding/purchases. Keep the existing notice on public/account/order-preview pages and leave their fixtures/data wiring unchanged. The admin overview can link to the working pages without claiming orders are enabled.

For Vite, instruct operators to authorize at the Go gate using the **same hostname** as the frontend (`localhost` on both, not mixed with `127.0.0.1`). Host-only cookies work across ports; the API proxy preserves Host for origin validation. An access-required panel links to the Go `/admin` gate (default backend port 8080), then tells the operator to return/reload the editor. No frontend key field, local-storage credential, or development bypass is added. With Vite alone, show backend-unavailable status rather than mutable fixtures.

Alternative: add a routing or form/state library and new `/admin/.../:id` pages. Existing query selectors and controlled React forms meet this bounded requirement without expanding dependencies or deep-link matching.

### 8. Explicit seed mode and bounded acceptance

Add a seed-only mode to the existing binary, `--seed-catalog --seed-at <RFC3339>`. It connects using `DATABASE_URL`/`DATABASE_SCHEMA`, acquires exclusive writer ownership, applies PostgreSQL migrations, loads embedded media, applies the seed transaction, prints a concise result, and exits without starting the engine, listener, or tunnel. Normal startup never seeds. Seed and import/verify modes are mutually exclusive. Require the reference instant explicitly; reject `--seed-at` without seed mode. It is an operator CLI, not a public HTTP reset endpoint.

Seed version 1 creates stable IDs, 18 distinguishable lots (six per event), all five categories, truthful illustrative stories/condition and media labels, USD cents/increments, and disclosed delivery/pickup terms. Event openings are reference +24h, +48h, +72h, each first close one hour later with 60-second staggering. Create all as drafts so publication uses the normal UI/API validation; rerun returns `already_applied` from the marker without overwriting edited rows or rebasing dates. Any non-seed identity/slug collision or insertion failure rolls back the entire seed. No ordinary startup seed or reset command. Refreshing expired demos requires editing still-draft schedules or explicitly choosing a new disposable PostgreSQL schema; published reset tooling remains Change 7.

Implement meaningful unit/HTTP/storage coverage against real PostgreSQL for money units, schedule math/range, schema/import/reconnect, atomic publish/failure/concurrency, membership, public-field exclusion, authorization/origin, and repeat-safe seed. Test explicit old-game reset isolation. Reuse existing frontend tests for route/query selection, copy completeness, money and server-error behavior rather than snapshotting form implementation.

During apply, run the relevant new checks, then `make test`, `make test-race`, and `make build` once after final edits. Use the built binary with an isolated PostgreSQL schema to seed, edit a lot, prepare/reorder/publish an event, inspect anonymous public JSON, and restart. Exercise missing/wrong cookie requests directly. Inspect EN/VI, keyboard-only controls, 320/375/768/1280 widths, 200 percent text zoom, local image failures, and offline media; record concise results/screenshots under this change's `verification/`. Do not claim realtime auction acceptance from publication tests.

Alternative: broad E2E infrastructure or production-scale load/security work. Targeted service/HTTP tests and built-app acceptance match the demo's size and core invariants; preserve existing game tests without extending the roadmap.

### 9. Exclusive game ownership and explicit offline SQLite import

The retained game is an in-memory authoritative engine and cannot become multi-process merely because PostgreSQL accepts many connections. Acquire a schema-specific **session advisory lock** on a dedicated `*sql.Conn` before migrations or game load. Every legacy game mutation uses that same connection; do not return it to the pool while ownership is held. A second server, importer, or seed command for the schema fails fast. Losing the connection makes game commits fail and stops the listener/tickers through existing fatal handling; never transparently reacquire a new lease and continue with the old in-memory snapshot. Catalog requests use pooled connections plus their transaction lock. Readiness probes check the retained ownership connection as well as the ordinary pool. This protocol is for one app process per schema, not a distributed-game architecture.

Add operator modes to the existing binary:

| Mode | Behavior |
|---|---|
| Normal (no maintenance flag) | PostgreSQL configuration/probe → writer lease → migrations → existing game/capability bootstrap → game load → listener |
| `--import-sqlite <snapshot-path>` | Read-only source preflight → PostgreSQL lease/migrations → unused-target check → import/verify/commit → exit |
| `--import-sqlite <snapshot-path> --verify-only` | Compare an existing target with the snapshot; read-only on both databases; no lease-taking write work, migrations, bootstrap, sequence changes, seed, engine, or listener |
| `--seed-catalog --seed-at <RFC3339>` | PostgreSQL lease/migrations → catalog seed transaction → exit |

`--verify-only` without import and incompatible flag combinations fail. Maintenance commands receive connection configuration via environment, not a URL CLI argument. Keep secrets out of report files and shell examples. No HTTP import/reset endpoint is added.

The supported source is a stopped, consistent **SQLite schema v4** snapshot with the eight expected application tables. Open using read-only SQLite mode and a read transaction without the old writable WAL-assertion `store.Open`. Check version, required columns, integrity, foreign keys, and representable types/values; reject embedded NUL/invalid text, invalid references, incompatible schema, or exhausted ID range rather than silently coercing them. Older snapshots must first be upgraded by the previous compatible SQLite binary on a separate working copy, then snapshotted; unknown/future versions are rejected. There is no previously implemented v5 catalog data to convert in this repository.

Use a cleanly stopped source with completed WAL checkpoint, or a consistent SQLite backup containing all committed WAL content. Never copy only the main file while the old app is writing, discard sidecars containing commits, or use an immutable-file shortcut on a live WAL database. Source snapshots/required sidecars are prepared before opening them read-only; import must not alter their bytes. Inventory media separately: the DB contains paths, not media blobs, so retain/copy the operator image directory as part of cutover. Do not read, print, or modify the user's live DB during this planning workflow.

Initialize target DDL/reference categories first, but require **all mutable application tables** empty, including game, players, items, bids, runtime, events, server configuration, catalog content, and seed/import markers. A freshly bootstrapped normal server is therefore an already-used target and is refused. Insert source rows in foreign-key order using explicit columns and IDs; retain raw legacy JSON text and NULL distinctions. Import never generates a new admin key or changes game money/currency. For bids and events, capture both `MAX(id)` and `sqlite_sequence.seq`; restart target identities above the higher high-water mark. Start at 1 only when neither source rows nor prior sequence history establish a higher value, including when every prior row was deleted.

Data insertion, typed-row verification, sequence restart, and the import marker commit in **one PostgreSQL transaction**. Use transactional identity/sequence `RESTART`, not `setval`, so failure can roll back sequence changes; [ALTER SEQUENCE documentation](https://www.postgresql.org/docs/18/sql-altersequence.html) distinguishes these behaviors. Only trusted fixed sequence identifiers and checked integer values enter that DDL. The import's source transaction remains open for its complete consistent snapshot. On any failure rollback the target transaction and report no completed import; migrated empty DDL/categories may remain for retry.

Verification computes row counts and SHA-256 digests over canonical typed values in primary-key order for all eight tables, distinguishing NULL, empty text, and raw JSON whitespace. Compare actual target rows before commit and again using independent `--verify-only` before normal startup. Include source version, logical source fingerprint, per-table counts/digests, foreign-key validity, and identity readiness without row values/tokens/keys. Require the same fixed snapshot and quiescent target for cutover verification. An identical completion marker reports `already_imported` without overwriting or pretending to reverify; a different source or nonempty target fails. Verification-only always checks actual current rows and reports drift, even if a marker exists. After normal boot, legacy deadline settlement may legitimately change the target; import validation occurs beforehand.

Alternative: automatic startup import or dual-write transition. Rejected because auto-bootstrap can mint replacement state and live copy cannot provide the required lossless cutover. Offline operator import matches the bounded demo scope and retains an unmodified rollback source.

### 10. PostgreSQL health, test isolation, and operational guidance

Change `/api/health` from SQLite's cached WAL assertion to a bounded current readiness probe. Success is HTTP 200 with `{status: "ok", database: "postgresql", ready: true, schema_version: 1}`. An unavailable DB/ownership connection returns HTTP 503 with sanitized `{status: "unavailable", database: "postgresql", ready: false}` while the process remains available to serve probes; a terminated process simply becomes unreachable. Remove `wal`, never synthesize `wal: true`, and never emit URLs, passwords, schema contents, or raw DB errors. Update health tests and scripts that relied on the old exact shape. Database errors from a catalog operation return an appropriate sanitized failure (503 if connectivity is known unavailable); an uncertain commit must be reloaded before retry. SQLSTATE 40001/40P01 produces a recoverable conflict after rollback; do not replay game callbacks or blindly retry operations with an uncertain commit.

Add a shared PostgreSQL test helper driven by `TEST_DATABASE_URL`, creating a generated isolated schema per test/fixture and cleaning up only that schema after connections/leases close. The test role needs schema creation privileges only in its dedicated test database. Pin schema configuration for all pool connections; never mutate a production/default schema for tests. Port temporary-file fixtures in store, game, hub, and server tests, and `scripts/loadtest-acceptance.sh`/Makefile harnesses, including tunnel drills. Keep source-v4 SQLite fixtures only for import tests; replace SQLite pragma/dialect tests with PostgreSQL connection/ledger/readiness/constraint tests. Required DB tests fail clearly if the service/config is absent; they must not silently skip or use SQLite. Run multi-connection and game commit/publish/restart/ordering coverage against PostgreSQL, not just SQL mocks.

Document PostgreSQL 18 provisioning, a loopback local/offline setup, app/test roles and schemas, `DATABASE_URL`/`DATABASE_SCHEMA`/`TEST_DATABASE_URL`, maintenance commands, and `pg_dump`/`pg_restore` backup/restore. The [PostgreSQL SQL dump guide](https://www.postgresql.org/docs/18/backup-dump.html) is the backup reference. An existing PostgreSQL service is sufficient; no Docker app deployment, root install, cloud account, or managed database is required by the design. Update active `ROADMAP.md`, `PROJECT.md`, `PLAN.md`, relevant `AGENTS.md` stack claims, and README references during apply, preserving archived evidence as historical. This planning revision edits only this change's artifacts.

## Risks / Trade-offs

- [A separate DB replaces the former self-contained persistence] → Keep one application binary, provide local PostgreSQL setup/offline rehearsal, and make DB readiness an explicit prerequisite.
- [Runtime port regresses retained game behavior] → Port every repository/test harness, preserve text/units/order/nulls, and verify commit-before-publish, crash/restart, and secret projections on real PostgreSQL.
- [Source copy omits committed WAL data] → Stop/checkpoint or create a consistent backup before import; verify typed rows against that fixed read-only snapshot.
- [Old and new writers diverge after cutover] → Stop the old writer, enforce PostgreSQL process ownership, and preserve both stores rather than claim reverse synchronization.
- [Published catalog needs correction] → Keep the existing frozen-publication policy; correct drafts before publishing or rehearse in a new disposable schema.
- [Media disappears or is publicly guessable] → Validate media at publish, retain fallback, preserve operator files during cutover, and document public media storage.
- [Seed dates become stale] → Keep stored dates; edit drafts or seed a new disposable schema deliberately.
- [Advisory locks serialize catalog writers] → Intentional for this demo; pooled readers remain concurrent and correctness is measured across separate connections.
- [Proxy development auth differs from embedded app] → Retain same-hostname cookie guidance and explicit Host/origin tests without permissive CORS.
- [Planning files are ignored] → Deliver exact local paths; leave staging/ignore rules unchanged.

## Migration Plan

1. During apply, implement and test the PostgreSQL runtime, ledger, importer, catalog, and harness changes using a dedicated test database. Reconcile active SQLite-only setup documents; do not modify archived implementation evidence.
2. Provision a separate PostgreSQL 18 instance/role and unused application schema; retain the previous working SQLite binary, a consistent source-v4 snapshot and required media. Stop the old app for the cutover rehearsal.
3. Run `--import-sqlite <snapshot>` before any normal server startup/seed against that target. Require successful in-transaction verification and a committed import marker. Run independent `--verify-only` and inspect the credential-free report. Exercise rollback/failure with deliberately invalid inputs in separate disposable schemas.
4. Start the PostgreSQL app only after verification, retaining `ADMIN_KEY` precedence intentionally. Confirm the persisted key when no override is set, readiness, legacy resume/wall-clock behavior, game revision/order, and absence of secret leaks. Treat expected legacy settle-at-boot changes separately from preboot migration equality.
5. Seed and prepare/publish catalog content in a separate acceptance schema (or on the verified target after import); inspect public projections, exact cents, image availability, concurrency, and restart. Run the frontend/offline/accessibility acceptance from Decision 8 and capture evidence under `verification/`.
6. Back up PostgreSQL and rehearse restore into a disposable target. Before public cutover, rollback can simply stop the new app and use the prior binary/source snapshot. After PostgreSQL accepts writes, preserve its backup first and explicitly acknowledge any writes lost by resuming the old SQLite snapshot; no automatic reverse migration or lossless post-write rollback is promised. Never run both apps as active writers for the same demo.

No blocking design questions remain. This is a proposed implementation/migration plan; no database was connected, imported, installed, or changed by this artifact revision.

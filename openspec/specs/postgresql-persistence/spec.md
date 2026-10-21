# postgresql-persistence Specification

## Purpose

Run AuctionLab on PostgreSQL with explicit database lifecycle, preserve existing SQLite data through a verified offline import, and maintain durable game and catalog semantics after cutover.

## Requirements

### Requirement: PostgreSQL-only runtime configuration
Normal server and catalog seed modes SHALL use PostgreSQL 18 through required `DATABASE_URL`, with an optional explicit application schema. Missing or invalid configuration, unsupported schema versions, or an unreachable database SHALL prevent startup. Runtime `DB_PATH` SHALL be rejected with migration guidance. SQLite SHALL be available only as an explicit import source, without runtime fallback or dual writes.

#### Scenario: Start against PostgreSQL
- **WHEN** the app starts with a valid PostgreSQL connection and a compatible application schema
- **THEN** all game, configuration, and catalog reads/writes use that PostgreSQL schema
- **AND** no SQLite file is created or opened by normal startup

#### Scenario: Obsolete or unavailable configuration
- **WHEN** `DATABASE_URL` is missing/invalid, runtime `DB_PATH` is set, or PostgreSQL is unavailable
- **THEN** startup fails with actionable sanitized diagnostics before accepting traffic
- **AND** neither database credentials nor a fallback SQLite database is exposed or created

### Requirement: Versioned PostgreSQL schema
PostgreSQL migrations SHALL have their own ordered version ledger and serialize concurrent schema changes. Fresh and upgraded schemas SHALL support retained game, catalog, accounts, sessions, watchlists, commerce lot auctions, accepted bids, immutable results, winner orders, simulated payment receipts, fulfillment progress/history, disposable-demo environment metadata and private interaction/coverage records. Repeated migration SHALL be safe; a failed step SHALL NOT be recorded as applied, and a newer-than-supported schema SHALL be refused. SQLite source versions SHALL NOT become PostgreSQL migration versions.

#### Scenario: Fresh and repeated migration
- **WHEN** schema initialization runs against a fresh PostgreSQL application schema and runs again
- **THEN** all required tables, constraints, and five catalog categories exist exactly once
- **AND** existing data and the recorded migration version remain consistent

#### Scenario: Failed or unsupported schema change
- **WHEN** a migration fails or the target ledger is newer than the app supports
- **THEN** initialization fails without a partial applied-version claim
- **AND** existing committed application data is retained

#### Scenario: Upgrade the Change 3 database
- **WHEN** the supported previous PostgreSQL schema is upgraded
- **THEN** account/session/watchlist storage becomes available without rewriting existing catalog content, game identities/amounts/revisions, publication state, or admin capability
- **AND** no accounts or watchlist entries are inferred from legacy players or created automatically

#### Scenario: Upgrade schema 2 for bidding
- **WHEN** a version-2 database applies bidding migration 3, checkout migration 4, fulfillment migration 5 and interaction migration 6 and migration runs again
- **THEN** new commerce auction storage exists once, previous migrations remain unchanged, and accounts/sessions/watchlists/catalog/game/admin capability are preserved
- **AND** migration alone creates no bids or results and readiness reports supported schema version 6

#### Scenario: Upgrade schema 3 for checkout
- **WHEN** a version-3 database applies migrations 4–6 and migration runs again
- **THEN** orders/payment storage and constraints exist exactly once, readiness reports supported version 6, and migrations 1–3 remain unchanged
- **AND** existing accounts, sessions, watchlists, published terms, accepted bids, auction results, admin capability and game data/revisions are preserved
- **AND** migration alone creates no order or payment; normal startup subsequently adopts existing sold results

#### Scenario: Checkout migration rolls back
- **WHEN** migration 4 fails before commit
- **THEN** the ledger remains at version 3 without partially installed checkout tables
- **AND** previously committed application data remains intact for correction and retry

#### Scenario: Upgrade schema 4 for fulfillment
- **WHEN** a populated version-4 schema applies migrations 5–6 and migration runs again
- **THEN** fulfillment storage and indexes exist once, each paid order starts at paid progress using its original receipt time, and unpaid orders have no invented fulfillment
- **AND** migrations 1–4, order IDs/snapshots/selections/totals/receipts, accounts, sessions, catalog, bids/results, game data and capability are preserved; health reports supported schema version 6

#### Scenario: Fulfillment migration fails
- **WHEN** migration 5 or its historical paid-progress initialization fails before commit
- **THEN** the ledger remains at version 4 without partial fulfillment structures or backfilled records
- **AND** existing purchase records remain intact for correction and retry

#### Scenario: Upgrade schema 5 for interaction collection
- **WHEN** a populated version-5 schema applies migration 6 and migration runs again
- **THEN** interaction/coverage storage and historical bid/payment backfills plus current-watchlist snapshots exist once, with no invented page views or removed-watchlist history
- **AND** migrations 1–5 and all existing identities, sessions, catalog, bids/results, purchases, progress, demo generation, capability and game records/revisions remain intact; health reports supported schema version 6

#### Scenario: Interaction migration fails
- **WHEN** migration 6 or its historical reconstruction fails before commit
- **THEN** the ledger remains at version 5 without partial structures, coverage metadata or backfills
- **AND** previous application data remains intact for a deliberate correction and retry

### Requirement: Explicit read-only SQLite import
An operator-only import mode SHALL accept a consistent SQLite snapshot at source schema version 4 and copy all retained application tables into an initialized but otherwise unused PostgreSQL application schema. Source access SHALL be read-only, and import SHALL not run an engine, start HTTP/tunnel services, seed demo records, or execute auction settlement. Other source versions SHALL be rejected with preparation guidance.

#### Scenario: Import a stopped application's snapshot
- **WHEN** an operator supplies a validated SQLite-v4 snapshot and an unused PostgreSQL target
- **THEN** players, items, sealed bids, bids, auction runtime, game, events, and server configuration are imported
- **AND** source contents, schema version, and source database files remain unchanged

#### Scenario: Unsupported or invalid source
- **WHEN** the source version is not 4, integrity or references fail, or values cannot be represented without loss
- **THEN** import fails and identifies the affected table/field without exposing secret values
- **AND** it does not repair, normalize, or write the source

### Requirement: Lossless legacy data semantics
Import SHALL preserve IDs, nickname keys, tokens, budgets, values, raw legacy JSON text, nulls, timestamps, revisions, bid order, sealed submissions, runtime deadlines/extensions, and persisted admin capability. Existing game amounts SHALL keep their original units. Subsequent generated IDs SHALL exceed imported IDs and the source's historical sequence high-water marks.

#### Scenario: Verify a populated source
- **WHEN** a source includes Unicode nicknames, nullable mechanics, a live runtime, sealed bids, and an admin key
- **THEN** PostgreSQL contains equivalent typed values and references for every source row
- **AND** the admin key and tokens still authorize through their existing boundaries without appearing in verification output

#### Scenario: Generate IDs after import
- **WHEN** a source has deleted high-numbered bid/event rows and a new bid/event is later inserted in PostgreSQL
- **THEN** its generated ID exceeds both the imported maximum and the saved SQLite sequence high-water mark
- **AND** history ordering and uniqueness remain correct

### Requirement: Atomic repeat-safe import and verification
Import SHALL commit its data and completion marker together, reject unrelated populated targets, and never overwrite newer target data. Repeating a completed import SHALL NOT duplicate rows. A separate verification-only mode SHALL compare the source with PostgreSQL using counts and canonical typed row digests, check sequence readiness, report mismatches, and perform no database writes.

#### Scenario: Interrupted import
- **WHEN** import fails before commit
- **THEN** no source application rows or completion marker are partly committed to the target
- **AND** an operator can correct the problem and retry against the unused target

#### Scenario: Repeat or populated target
- **WHEN** the same import is repeated or a different import targets an already-used schema
- **THEN** the former reports previously applied without overwriting data and the latter is refused
- **AND** an existing completion marker is not reported as a new successful verification

#### Scenario: Detect content drift without writes
- **WHEN** verification-only compares a source snapshot to a target with a changed amount or missing row
- **THEN** it exits unsuccessfully with a table/count/digest mismatch summary
- **AND** source/target data, sequence state, and migration metadata remain unchanged

#### Scenario: Account data occupies the target
- **WHEN** a first-time SQLite import targets a migrated schema containing commerce accounts, sessions, watchlist entries, an account-seed marker, commerce auctions, accepted commerce bids or auction results
- **THEN** it refuses the occupied target before copying any source row
- **AND** account data and the read-only SQLite source remain unchanged

### Requirement: Preserve game persistence and process ownership
PostgreSQL storage SHALL retain commit-before-publish, monotonic game revisions, nickname uniqueness, bid ordering, atomic budget changes, and wall-clock restart behavior. One server process SHALL own a given application schema's in-memory game writer. Another server or write-mode maintenance command SHALL be refused while that owner is active. Loss of writer ownership SHALL fail closed.

#### Scenario: Resume migrated game state
- **WHEN** the migrated server starts from verified runtime data, including an expired legacy deadline
- **THEN** it resumes or settles using the existing game rules and actual wall-clock time
- **AND** no successful game update is published before its transaction commits

#### Scenario: Competing process or lost owner connection
- **WHEN** a second writer starts for the same schema or the first writer's ownership connection fails
- **THEN** the competing writer is refused while ownership is held, or the disconnected writer stops accepting successful mutations
- **AND** stale in-memory state cannot continue to overwrite committed game data

### Requirement: Database concurrency replaces SQLite connection serialization
Correctness SHALL hold with multiple PostgreSQL pool connections. Catalog draft edits, membership changes, and publication SHALL coordinate transactionally and reject stale versions. Public aggregate reads SHALL use a consistent committed snapshot. Database failures SHALL NOT produce partial publication or a false saved-state response.

#### Scenario: Concurrent catalog operations
- **WHEN** multiple connections attempt lot edits, assignments, or publication for the same prepared data
- **THEN** committed state satisfies exclusive assignment, completeness, expected-version, and immutability rules
- **AND** conflicts return a recoverable response rather than silently overwriting data

#### Scenario: Aggregate read during publication
- **WHEN** public reads overlap the publication commit
- **THEN** each response reflects one coherent committed event and membership set
- **AND** no response reveals a partly published aggregate

### Requirement: PostgreSQL readiness without credential leakage
`GET /api/health` SHALL report PostgreSQL identity, current readiness, and the known schema version without SQLite WAL fields or credentials. It SHALL return HTTP 200 only when the database is reachable and the schema is ready, and HTTP 503 when readiness fails. Database diagnostics and public errors SHALL redact connection credentials and sensitive row values.

#### Scenario: Ready application
- **WHEN** PostgreSQL is reachable with a compatible migrated schema
- **THEN** health returns JSON identifying PostgreSQL and `ready: true` with HTTP 200
- **AND** it contains no `wal` field, connection string, password, or capability value

#### Scenario: Database becomes unavailable
- **WHEN** PostgreSQL cannot answer the bounded readiness probe
- **THEN** health returns sanitized JSON with `ready: false` and HTTP 503 while the server is still serving probes
- **AND** no catalog mutation claims a successful commit during the outage

### Requirement: Recoverable cutover and separate database service
The app SHALL retain a no-CGO binary containing the frontend, while requiring a separate PostgreSQL service. Operator guidance SHALL cover local/offline operation, provisioning, stopped-writer source snapshots, verification before cutover, backup/restore, and rollback. Rolling back to SQLite SHALL explicitly acknowledge that post-cutover PostgreSQL writes are not automatically synchronized back.

#### Scenario: Offline demonstration
- **WHEN** the app, local PostgreSQL service, and media are available with external internet disconnected
- **THEN** catalog preparation and persistence continue without a cloud database dependency
- **AND** the operator can identify PostgreSQL as a required separate process

#### Scenario: Roll back after a rehearsal
- **WHEN** the operator follows the documented rollback drill
- **THEN** the PostgreSQL app is stopped and the preserved SQLite snapshot is opened only with the previous compatible binary
- **AND** post-cutover PostgreSQL data is preserved separately and any discarded writes are disclosed

### Requirement: Real PostgreSQL acceptance
Database-backed repository, game, hub, server, migration, publication, account, watchlist, timed bidding, closure, auction recovery, order history, fulfillment and demo-maintenance checks SHALL run against isolated real PostgreSQL schemas. Test and load harnesses SHALL use explicit disposable targets, clean up only their own resources, and fail clearly when required PostgreSQL is unavailable. SQLite import fixtures SHALL remain limited to source-compatibility tests.

#### Scenario: Required database suite
- **WHEN** full verification runs with an isolated PostgreSQL test service
- **THEN** legacy game, catalog, account, and watchlist correctness checks run against PostgreSQL with multiple pool connections
- **AND** a missing test service does not silently skip required acceptance or substitute SQLite

#### Scenario: Durable ownership and restart acceptance
- **WHEN** account/watchlist checks register two users, save independently, revoke a session, and reopen the same owned test schema
- **THEN** account identity and each watchlist persist, valid unexpired sessions still resolve, and revoked sessions remain invalid
- **AND** fixture cleanup touches only schemas created by that run

#### Scenario: Purchase operations and isolated reset
- **WHEN** acceptance exercises concurrent transitions, payment/fulfillment failure recovery, restart and demo reset on owned disposable schemas
- **THEN** purchase invariants and owner/operator access remain correct using real PostgreSQL
- **AND** tests prove refused operational targets and unchanged other-schema records rather than using SQLite or skipping missing fixtures

### Requirement: Account writes respect database ownership and commit boundaries
Account/session/watchlist writes and account provisioning SHALL honor the existing schema writer lease and database timeouts. Lost ownership SHALL fail closed without reacquiring a writer for stale process state. Failure or uncertain commit SHALL NOT be reported as confirmed success, and account mutations SHALL NOT change game revisions or publication state.

#### Scenario: Lost writer or database outage
- **WHEN** schema ownership is lost or PostgreSQL fails during registration, login, logout, profile update, or membership change
- **THEN** the operation returns a sanitized failure or uncertain-result response and issues no newly successful session response
- **AND** the process does not reacquire ownership to continue with stale memory

#### Scenario: Watchlist operation commits
- **WHEN** a save or remove response reports success
- **THEN** a subsequent authenticated read observes the committed membership
- **AND** game revisions, catalog publication, and other accounts' memberships remain unchanged
### Requirement: Commerce auction commit and ownership safety
Commerce bid acceptance, initialization and terminal results SHALL honor the schema writer lease, database timeouts and commit-before-success boundary. Lost ownership SHALL fail closed without reacquisition. Accepted bids/results SHALL survive restart; game resets SHALL NOT remove them. Auction changes SHALL NOT modify game revisions or immutable publication.

#### Scenario: Failed bid or terminal commit
- **WHEN** a bid or closure transaction fails before commit
- **THEN** no partial accepted history, leader, count, revision or result is committed or announced
- **AND** an uncertain commit is reported as unknown and is reconciled from durable records instead of claimed as success

#### Scenario: Writer ownership lost
- **WHEN** the leased writer connection fails during auction execution
- **THEN** further bid and lifecycle mutations stop succeeding and readiness reports unavailable
- **AND** the process does not reacquire ownership to continue from stale process state

#### Scenario: Restart and legacy reset
- **WHEN** the server restarts around a closing deadline or the retained game is reset
- **THEN** accepted commerce bids and committed terminal results remain intact and overdue lots reconcile exactly once
- **AND** catalog publication, commerce identities, watchlists and game monetary units remain independent

### Requirement: Checkout ownership and commit safety
Order creation, result adoption, simulated confirmation and fulfillment transitions SHALL honor the schema writer lease, operation timeouts and commit-before-success boundary. Lost ownership SHALL fail closed without reacquisition. Orders/receipts/fulfillment history SHALL survive restart and retained game resets without changing catalog publication, auction results or game monetary units/revisions.

#### Scenario: Failed or uncertain checkout commit
- **WHEN** result/order creation or payment confirmation fails or cannot establish commit
- **THEN** no partial operation is reported as successful; uncertain payment is recoverable through durable owner reads
- **AND** loss of the writer lease stops further writes and makes readiness unavailable

#### Scenario: Reset does not erase purchases
- **WHEN** the retained classroom game is reset or the server restarts
- **THEN** orders, immutable snapshots, simulated receipts and fulfillment progress/history remain intact
- **AND** restarted reconciliation creates no duplicate order or payment

#### Scenario: Fulfillment commit and lease loss
- **WHEN** fulfillment fails before commit, has an unknown commit, or loses writer ownership
- **THEN** no partial history/status is reported as successful and durable detail reads support reconciliation
- **AND** the stale process stops successful writes and does not reacquire the writer lease

### Requirement: Import occupancy includes purchase records
The read-only SQLite importer SHALL treat order, simulated-payment, fulfillment, disposable-demo environment, interaction and interaction-coverage records as target occupancy. Import SHALL NOT overwrite or merge purchase data, adopt winners, create accounts, run payment simulation or invent commerce interactions.

#### Scenario: Import into an occupied purchase schema
- **WHEN** the operator supplies an initialized target containing an order, simulated-payment, fulfillment, disposable-demo environment, interaction or collection-coverage record
- **THEN** import refuses the target without changing source or destination records
- **AND** diagnostics disclose no connection credentials or checkout contact/address data

### Requirement: Interaction ownership and aggregate acceptance
Interaction writes SHALL honor the leased schema writer and commit-before-success boundary. Aggregate/recommendation reads SHALL use consistent read-only snapshots. Real-PostgreSQL acceptance SHALL verify atomic action capture, historical reconstruction, deduplication, privacy, exact aggregate values, restart and disposable reset without SQLite fallback.

#### Scenario: Snapshot and writer safety
- **WHEN** collection overlaps commerce commits or the writer lease is lost
- **THEN** aggregate responses remain consistent and further writes fail closed without reacquiring ownership
- **AND** no interaction write changes game revisions, publication terms or prior immutable purchase facts

#### Scenario: Required isolated database checks
- **WHEN** implementation acceptance runs or its test PostgreSQL service is missing
- **THEN** repository/API/migration checks use owned disposable schemas or fail clearly for the missing service
- **AND** tests never use operational schemas or substitute SQLite for the required PostgreSQL checks

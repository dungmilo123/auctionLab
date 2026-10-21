# Spec Delta

## MODIFIED Requirements

### Requirement: Versioned PostgreSQL schema
PostgreSQL migrations SHALL have their own ordered version ledger and serialize concurrent schema changes. Fresh and upgraded schemas SHALL support retained game, catalog, accounts, sessions, watchlists, commerce lot auctions, accepted bids, immutable results, winner orders, simulated payment receipts, fulfillment progress/history and disposable-demo environment metadata. Repeated migration SHALL be safe; a failed step SHALL NOT be recorded as applied, and a newer-than-supported schema SHALL be refused. SQLite source versions SHALL NOT become PostgreSQL migration versions.

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
- **WHEN** a version-2 database applies bidding migration 3, checkout migration 4 and fulfillment migration 5 and migration runs again
- **THEN** new commerce auction storage exists once, previous migrations remain unchanged, and accounts/sessions/watchlists/catalog/game/admin capability are preserved
- **AND** migration alone creates no bids or results and readiness reports supported schema version 5

#### Scenario: Upgrade schema 3 for checkout
- **WHEN** a version-3 database applies migrations 4–5 and migration runs again
- **THEN** orders/payment storage and constraints exist exactly once, readiness reports supported version 5, and migrations 1–3 remain unchanged
- **AND** existing accounts, sessions, watchlists, published terms, accepted bids, auction results, admin capability and game data/revisions are preserved
- **AND** migration alone creates no order or payment; normal startup subsequently adopts existing sold results

#### Scenario: Checkout migration rolls back
- **WHEN** migration 4 fails before commit
- **THEN** the ledger remains at version 3 without partially installed checkout tables
- **AND** previously committed application data remains intact for correction and retry

#### Scenario: Upgrade schema 4 for fulfillment
- **WHEN** a populated version-4 schema applies migration 5 and migration runs again
- **THEN** fulfillment storage and indexes exist once, each paid order starts at paid progress using its original receipt time, and unpaid orders have no invented fulfillment
- **AND** migrations 1–4, order IDs/snapshots/selections/totals/receipts, accounts, sessions, catalog, bids/results, game data and capability are preserved; health reports supported schema version 5

#### Scenario: Fulfillment migration fails
- **WHEN** migration 5 or its historical paid-progress initialization fails before commit
- **THEN** the ledger remains at version 4 without partial fulfillment structures or backfilled records
- **AND** existing purchase records remain intact for correction and retry

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
The read-only SQLite importer SHALL treat order, simulated-payment, fulfillment and disposable-demo environment records as target occupancy. Import SHALL NOT overwrite or merge purchase data, adopt winners, create accounts, or run payment simulation.

#### Scenario: Import into an occupied purchase schema
- **WHEN** the operator supplies an initialized target containing an order, simulated-payment, fulfillment or disposable-demo environment record
- **THEN** import refuses the target without changing source or destination records
- **AND** diagnostics disclose no connection credentials or checkout contact/address data

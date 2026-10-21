# Spec Delta

## MODIFIED Requirements

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

### Requirement: Import occupancy includes purchase records
The read-only SQLite importer SHALL treat order, simulated-payment, fulfillment, disposable-demo environment, interaction and interaction-coverage records as target occupancy. Import SHALL NOT overwrite or merge purchase data, adopt winners, create accounts, run payment simulation or invent commerce interactions.

#### Scenario: Import into an occupied purchase schema
- **WHEN** the operator supplies an initialized target containing an order, simulated-payment, fulfillment, disposable-demo environment, interaction or collection-coverage record
- **THEN** import refuses the target without changing source or destination records
- **AND** diagnostics disclose no connection credentials or checkout contact/address data

## ADDED Requirements

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

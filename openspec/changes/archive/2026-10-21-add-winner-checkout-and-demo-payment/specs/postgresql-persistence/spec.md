# Spec Delta

## MODIFIED Requirements

### Requirement: Versioned PostgreSQL schema
PostgreSQL migrations SHALL have their own ordered version ledger and serialize concurrent schema changes. Fresh and upgraded schemas SHALL support retained game, catalog, accounts, sessions, watchlists, commerce lot auctions, accepted bids, immutable results, winner orders and simulated payment receipts. Repeated migration SHALL be safe; a failed step SHALL NOT be recorded as applied, and a newer-than-supported schema SHALL be refused. SQLite source versions SHALL NOT become PostgreSQL migration versions.

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
- **WHEN** a version-2 database applies bidding migration 3 and checkout migration 4 and migration runs again
- **THEN** new commerce auction storage exists once, previous migrations remain unchanged, and accounts/sessions/watchlists/catalog/game/admin capability are preserved
- **AND** migration alone creates no bids or results and readiness reports supported schema version 4

#### Scenario: Upgrade schema 3 for checkout
- **WHEN** a version-3 database applies migration 4 and migration runs again
- **THEN** orders/payment storage and constraints exist exactly once, readiness reports supported version 4, and migrations 1–3 remain unchanged
- **AND** existing accounts, sessions, watchlists, published terms, accepted bids, auction results, admin capability and game data/revisions are preserved
- **AND** migration alone creates no order or payment; normal startup subsequently adopts existing sold results

#### Scenario: Checkout migration rolls back
- **WHEN** migration 4 fails before commit
- **THEN** the ledger remains at version 3 without partially installed checkout tables
- **AND** previously committed application data remains intact for correction and retry

## ADDED Requirements

### Requirement: Checkout ownership and commit safety
Order creation, result adoption and simulated confirmation SHALL honor the schema writer lease, operation timeouts and commit-before-success boundary. Lost ownership SHALL fail closed without reacquisition. Orders/receipts SHALL survive restart and retained game resets without changing catalog publication, auction results or game monetary units/revisions.

#### Scenario: Failed or uncertain checkout commit
- **WHEN** result/order creation or payment confirmation fails or cannot establish commit
- **THEN** no partial operation is reported as successful; uncertain payment is recoverable through durable owner reads
- **AND** loss of the writer lease stops further writes and makes readiness unavailable

#### Scenario: Reset does not erase purchases
- **WHEN** the retained classroom game is reset or the server restarts
- **THEN** orders, immutable snapshots and simulated receipts remain intact
- **AND** restarted reconciliation creates no duplicate order or payment

### Requirement: Import occupancy includes purchase records
The read-only SQLite importer SHALL treat order and simulated-payment records as target occupancy. Import SHALL NOT overwrite or merge purchase data, adopt winners, create accounts, or run payment simulation.

#### Scenario: Import into an occupied purchase schema
- **WHEN** the operator supplies an initialized target containing an order or simulated-payment record
- **THEN** import refuses the target without changing source or destination records
- **AND** diagnostics disclose no connection credentials or checkout contact/address data

# Spec Delta

## MODIFIED Requirements

### Requirement: Versioned PostgreSQL schema
PostgreSQL migrations SHALL have their own ordered version ledger and serialize concurrent schema changes. Fresh and upgraded schemas SHALL support retained game, catalog, accounts, sessions, and watchlists. Repeated migration SHALL be safe; a failed step SHALL NOT be recorded as applied, and a newer-than-supported schema SHALL be refused. SQLite source versions SHALL NOT become PostgreSQL migration versions.

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
- **WHEN** a first-time SQLite import targets a migrated schema containing commerce accounts, sessions, watchlist entries, or an account-seed marker
- **THEN** it refuses the occupied target before copying any source row
- **AND** account data and the read-only SQLite source remain unchanged

### Requirement: Real PostgreSQL acceptance
Database-backed repository, game, hub, server, migration, publication, account, and watchlist checks SHALL run against isolated real PostgreSQL schemas. Test and load harnesses SHALL use explicit disposable targets, clean up only their own resources, and fail clearly when required PostgreSQL is unavailable. SQLite import fixtures SHALL remain limited to source-compatibility tests.

#### Scenario: Required database suite
- **WHEN** full verification runs with an isolated PostgreSQL test service
- **THEN** legacy game, catalog, account, and watchlist correctness checks run against PostgreSQL with multiple pool connections
- **AND** a missing test service does not silently skip required acceptance or substitute SQLite

#### Scenario: Durable ownership and restart acceptance
- **WHEN** account/watchlist checks register two users, save independently, revoke a session, and reopen the same owned test schema
- **THEN** account identity and each watchlist persist, valid unexpired sessions still resolve, and revoked sessions remain invalid
- **AND** fixture cleanup touches only schemas created by that run

## ADDED Requirements

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

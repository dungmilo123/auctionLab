# demo-rehearsal Specification

## Purpose

Allow operators to prepare another complete auction-to-fulfillment demonstration in an explicitly disposable PostgreSQL schema while protecting ordinary application data.

## Requirements

### Requirement: Explicit disposable demo preparation
Stopped-writer `--prepare-demo --demo-schema <name> --seed-at <RFC3339>` SHALL prepare only a new unused `auctionlab_demo_<suffix>` schema and record a durable disposable-environment marker. It SHALL seed labeled accounts with operator-supplied passwords and 18 draft lots in three events. Existing populated targets without that marker SHALL be refused, never adopted as disposable.

#### Scenario: Prepare and repeat safely
- **WHEN** an operator supplies an eligible unused schema, seed reference and all required password environment inputs
- **THEN** preparation commits the environment marker, demo accounts and complete draft catalog content and exits without starting services
- **AND** repeating preparation on that marked prepared schema reports already prepared without resetting data, changing credentials or rebasing dates

#### Scenario: Invalid target or incomplete input
- **WHEN** the schema is the default application schema, lacks the dedicated prefix, contains unrelated records, or first preparation lacks valid inputs
- **THEN** preparation refuses before changing existing application records or marking the schema disposable
- **AND** no credentials are printed or supplied through command-line arguments

### Requirement: Explicit bounded demo reset
`--reset-demo --demo-schema <name> --confirm-demo-schema <same-name> --seed-at <RFC3339>` SHALL reset only a previously marked disposable schema. Prefix, marker, exact confirmation and exclusive writer ownership SHALL all be required. The command SHALL refuse ordinary/unmarked schemas even if records have demo labels. No HTTP reset endpoint SHALL exist.

#### Scenario: Reset a completed rehearsal
- **WHEN** the server is stopped and an operator supplies the exact marked demo schema, confirmation, fresh reference and required passwords
- **THEN** only that schema's rehearsal data is cleared and reseeded with fresh account identities and draft event schedules
- **AND** the output identifies the target and write scope without contact/address, passwords, session tokens or capability values

#### Scenario: Active writer or unsafe target
- **WHEN** reset targets a running schema, an unmarked schema, a non-demo-prefixed name, or a mismatched confirmation
- **THEN** it refuses without removing any data
- **AND** a populated operational schema and other schemas remain unchanged

### Requirement: Atomic documented reset write scope
Reset SHALL clear the target's sessions, accounts/watchlists, seed markers, catalog content, commerce auctions/bids/results, orders, simulated receipts and fulfillment history, plus retained game rehearsal data. It SHALL preserve schema migrations, the disposable marker and admin capability configuration. Clear/reseed SHALL commit atomically; no cross-schema cascade or external media deletion SHALL occur.

#### Scenario: Reset failure and retry
- **WHEN** clear/reseed fails before commit because of validation, collision, dependency or storage failure
- **THEN** the previous rehearsal records and credentials remain intact
- **AND** failure or unknown commit is not reported as a confirmed fresh rehearsal; the operator inspects durable environment generation before retry

#### Scenario: Other data remains protected
- **WHEN** a reset succeeds while another schema contains operational purchases and an images directory contains media
- **THEN** other-schema records and all media files are unchanged
- **AND** old demo session cookies cease to authenticate because the target's prior sessions/accounts were removed

### Requirement: Fresh workflow remains deliberate
Preparation/reset SHALL require an explicit valid seed reference and preserve the existing seed schedule offsets. It SHALL create drafts without publishing, bidding, choosing winners, paying or advancing fulfillment. Normal startup, existing account/catalog seed modes and the retained game reset SHALL NOT reset commerce or rebase immutable schedules.

#### Scenario: Rehearse from the start
- **WHEN** the operator starts the newly prepared demo schema
- **THEN** they can review/edit future draft schedules, publish, bid through two accounts, wait for closure, pay and advance shipping/pickup using normal authorized actions
- **AND** seeding alone creates no sold result, order, receipt or fulfillment event

### Requirement: Read-only rehearsal generation inspection
`--demo-status --demo-schema <name>` SHALL report the marked disposable environment's generation, seed reference and preparation time without changing data or starting services. Missing/unmarked targets SHALL be refused without initialization. Operators SHALL use this committed state to inspect an uncertain prepare/reset outcome before another deliberate reset.

#### Scenario: Lost reset response
- **WHEN** a reset response is lost and the operator runs demo status for the same target
- **THEN** it reports the currently committed generation and seed reference without modifying rehearsal records
- **AND** the operator can distinguish a committed replacement from the previous generation without exposing credentials or buyer data

### Requirement: Maintenance isolation and operator guidance
Demo modes SHALL use required PostgreSQL configuration, be mutually exclusive with each other and import/verify/existing seed/tunnel modes, and exit before HTTP, lifecycle, game or tunnel services start. Write modes SHALL require exclusive writer ownership. Documentation SHALL disclose erased/preserved data, backup guidance, password inputs, target selection and the complete replay drill.

#### Scenario: Invalid mode combination
- **WHEN** a demo mode is combined with another maintenance/tunnel mode or lacks its explicit target
- **THEN** the command rejects the combination before maintenance writes
- **AND** it does not fall back to the configured/default application schema

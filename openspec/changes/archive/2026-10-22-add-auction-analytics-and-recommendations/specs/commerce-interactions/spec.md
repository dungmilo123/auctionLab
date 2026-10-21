# Spec Delta

## Purpose

Persist the minimal first-party observations and authoritative commerce actions needed for honest analytics and possible later recommendation research.

## ADDED Requirements

### Requirement: Minimal interaction records
Interactions SHALL record only kind, lot/event identity, server occurrence time, provenance, derived demo flags and bounded deduplication/source references. Account identity SHALL exist only for server-authenticated actions. Anonymous views SHALL store no visitor identity, cookie, IP, user agent, referrer, query text or cross-page identifier. Records SHALL remain private to the application schema.

#### Scenario: Supported kinds and prohibited data
- **WHEN** the system records `lot_view`, `watchlist_add`, `bid_accepted` or `payment_confirmed`
- **THEN** only its allowlisted fields are persisted and no raw event export is available through public, account or analytics APIs
- **AND** passwords, tokens, capability values, contact/address, shipment fields, request bodies and financial credentials are neither stored in the record nor logged by collection

### Requirement: Anonymous page observations
`POST /api/catalog/lot-views` SHALL accept only a published `lot_id` and lowercase UUID `observation_id`, assigning server time and demo flags. It SHALL require strict JSON up to 1 KiB, `X-AuctionLab-Interaction: 1` and the existing same-origin policy. A view SHALL represent a successfully rendered lot detail document, not a card impression, API read or unique person.

#### Scenario: One page observation and retries
- **WHEN** a published lot detail renders, refreshes its auction state, changes language or repeats its view request
- **THEN** that document uses one in-memory observation ID and records at most one view
- **AND** a new document load can record another observation; no token is persisted in browser storage or linked across pages

#### Scenario: Invalid, unavailable and bounded requests
- **WHEN** the body is malformed/oversized, contains unknown fields or client kind/time/account data, targets a missing/draft lot, or violates request boundaries
- **THEN** the endpoint returns a sanitized 400/413, 404, 403 or 415 as appropriate, with 422 for invalid field shapes and no record
- **AND** an in-memory direct-peer limiter returns 429 above 30 attempts per minute and bounds active peer keys to 4096 without persisting peer addresses

#### Scenario: Collection failure is nonblocking
- **WHEN** view recording fails, is rate-limited, or has an unknown commit result
- **THEN** the catalog and deliberate commerce actions remain usable without a blocking error or automatic retry loop
- **AND** a repeated same-ID observation is harmless; reusing an ID for a different lot returns 409 and never rewrites its original binding

### Requirement: Atomic authoritative action capture
A newly inserted watchlist membership, accepted bid or first simulated payment SHALL create one matching interaction in the same transaction. Clients SHALL NOT submit these event kinds through the view endpoint. Removal SHALL leave historical additions intact, and an actual remove-then-add SHALL create a new addition event.

#### Scenario: Idempotent mutation capture
- **WHEN** a save is repeated without removal, an accepted bid request is replayed, or a paid confirmation is repeated using the same or a fresh request ID
- **THEN** only the original actual transition has an interaction
- **AND** rejected bids, stateless checkout reviews, failed confirmations and fulfillment changes create no bid/payment interaction

#### Scenario: Failure, rollback and unknown outcome
- **WHEN** action capture or its parent transaction fails before commit
- **THEN** both the commerce mutation and interaction roll back and no confirmed success is returned
- **AND** an uncertain commit retains existing durable recovery semantics; retry or restart creates no duplicate bid/payment interaction and never reacquires lost writer ownership

### Requirement: Honest historical collection coverage
Initial schema upgrade SHALL reconstruct accepted-bid and confirmed-payment interactions from their durable rows using original times and mark them backfilled. Current watchlist memberships SHALL be represented as snapshots, never complete historical additions. No views or removed-watchlist history SHALL be invented. Metadata SHALL identify collection start and provenance counts separately.

#### Scenario: Upgrade an existing journey
- **WHEN** an existing schema contains bids, paid receipts, a currently saved lot and no recorded views
- **THEN** those durable actions appear once as backfilled/snapshot records and observed views remain zero since collection began
- **AND** the dashboard distinguishes incomplete observed history from full-lifetime commerce metrics and makes no unique-visitor or conversion claims

#### Scenario: Restart and retained game reset
- **WHEN** the application restarts, migrations repeat or the retained game resets
- **THEN** interaction records and collection-start metadata persist without duplicate historical reconstruction
- **AND** ordinary boot never fabricates new activity or resets its collection baseline

### Requirement: View acknowledgement and read isolation
Successful first or duplicate view recording SHALL return 200 with an allowlisted recorded acknowledgement only after confirmed commit. Every response SHALL be no-store. Catalog, analytics and recommendation GET requests SHALL remain read-only; only the separate optional view POST and deliberate commerce mutations SHALL append interactions.

#### Scenario: Read and observation boundaries
- **WHEN** a bot fetches public JSON, an operator refreshes analytics or a client requests recommendations
- **THEN** those GETs create no interaction, account, membership, bid, order or payment
- **AND** only a separate accepted view POST records a page observation, without setting cookies or requiring an account

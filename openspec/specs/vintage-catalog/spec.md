# vintage-catalog Specification

## Purpose

Provide durable vintage catalog content and explicit public representations that future discovery and commerce features can consume without inheriting classroom-game semantics.

## Requirements

### Requirement: Vintage categories and lot content
The catalog SHALL provide film cameras, watches, vinyl records, audio equipment, and decorative objects as categories. Lots SHALL have stable IDs and unique slugs, a title, category, description, brand, era, condition, ordered images with alternative text, specifications, supplied story/provenance, fulfillment information, and optional admin notes. Unknown brand, era, or provenance SHALL be represented honestly.

#### Scenario: Save and reopen a lot
- **WHEN** an authorized operator saves a lot with multiple images, ordered specification name/value pairs, a condition description, delivery fee and instructions, and a supplied story
- **THEN** reopening the lot returns the entered content, category, image order, and specification order
- **AND** a supplied story is not automatically presented as independently verified authenticity

#### Scenario: Missing historical information
- **WHEN** an operator does not know a lot's brand, exact era, or provenance
- **THEN** those fields can remain absent
- **AND** saving or publication does not invent historical facts or authentication claims

### Requirement: Draft content and stable identity
A new lot SHALL be a draft with a required title and a generated unique slug. Drafts SHALL allow incomplete publication fields, while rejecting invalid supplied values. IDs SHALL remain stable; slugs SHALL be editable and unique before publication and immutable afterward. Categories SHALL be selectable from the supplied list without requiring a category editor.

#### Scenario: Start an incomplete draft
- **WHEN** an operator saves only a nonempty title
- **THEN** the catalog creates a durable draft with an ID and URL-safe slug
- **AND** missing images, category, fulfillment, or pricing block publication rather than draft saving

#### Scenario: Invalid supplied values
- **WHEN** an operator supplies a duplicate slug, unknown category, negative fee, or invalid image path
- **THEN** the save fails with an identifiable validation or conflict response
- **AND** the previously stored record remains unchanged

### Requirement: Explicit commerce money
Catalog money SHALL use currency `USD` and nonnegative integer cents in explicitly named `*_minor` fields, capped at 1,000,000,000,000 cents per field. Bid increments SHALL be positive when supplied. Decimal entry SHALL preserve cents exactly, reject more than two fractional digits and out-of-range values, and SHALL NOT reuse legacy game amounts as commerce prices.

#### Scenario: Enter a price with cents
- **WHEN** an operator enters an opening price of `125.50` and increment of `5.00`
- **THEN** the saved fields are `starting_price_minor: 12550`, `bid_increment_minor: 500`, and `currency: "USD"`
- **AND** reopening displays $125.50 and $5.00 without rounding or a hundredfold scaling error

#### Scenario: Invalid money and currency
- **WHEN** a request supplies fractional cents, an unsafe or over-limit integer, a negative price, zero increment, or a currency other than USD
- **THEN** the request is rejected without changing the record
- **AND** old game budgets and the game currency configuration remain untouched

### Requirement: Local catalog media
Catalog image references SHALL use application-origin paths under `/images/` or `/site-assets/catalog/`, without traversal, query strings, fragments, or external URLs. Publication SHALL require readable image files and nonempty alternative text for each referenced image, including the event cover. Seed media SHALL be bundled and have recorded source/license information. Missing images after publication SHALL retain an accessible local fallback.

#### Scenario: Local gallery and cover
- **WHEN** an operator publishes an event whose cover and lot gallery paths resolve to local supported image files
- **THEN** the media load from the application origin with external internet unavailable
- **AND** gallery order and alternative text are retained

#### Scenario: Unsafe or unavailable media
- **WHEN** an image reference is external, escapes an allowed namespace, or names a missing file at publication
- **THEN** the invalid reference is rejected or publication reports the missing image
- **AND** the server does not fetch remote media or publish a partially valid event

### Requirement: Public catalog projections
Public catalog list and slug-detail reads SHALL expose only published events and their published lots using an explicit public field set. That set SHALL include descriptions, media, fulfillment terms/fees, currency, opening price, increment, event membership/order, and planned opening/closing times. It SHALL exclude drafts, admin notes, internal version/seed metadata, credentials, and all legacy game secrets or player data.

#### Scenario: Discover published data through the API
- **WHEN** an anonymous client lists catalog events/lots or reads a published record by slug
- **THEN** the response contains the allowed published content and planned schedule
- **AND** it contains no fabricated current bid, winner, live state, or settlement result

#### Scenario: Draft cannot be discovered by guessing
- **WHEN** an anonymous client requests a draft or nonexistent slug
- **THEN** both return the same not-found response
- **AND** public list results exclude unassigned drafts and lots in draft events

#### Scenario: Private fields remain private
- **WHEN** published records contain distinctive admin-note text and the database also contains game truth values, sealed bids, and a persisted admin key
- **THEN** none of those values or private fields appears in public catalog JSON
- **AND** an admin can inspect the permitted public projection before publication without exposing the draft publicly

### Requirement: Durable independent catalog
Saved categories, lot content, assignment/order, publication state, and event schedules SHALL survive application and PostgreSQL restart. Catalog creation and SQLite import SHALL preserve legacy game records and admin capability state. Legacy game reset SHALL NOT delete or change the commerce catalog, and catalog operations SHALL NOT mutate game state or revision.

#### Scenario: Reopen the database
- **WHEN** the app reconnects after an application or PostgreSQL restart using a database containing draft and published catalog records
- **THEN** the records, IDs, amounts, media order, and stored dates remain unchanged
- **AND** public visibility continues to reflect the committed publication state

#### Scenario: Upgrade an existing game database
- **WHEN** a supported SQLite-v4 snapshot is imported and verified in PostgreSQL and its game is subsequently reset
- **THEN** the catalog remains available independently of the game
- **AND** the import preserves existing game rows, their original monetary units, and the stored admin capability

### Requirement: Explicit repeat-safe demo content
An explicit demo seed operation SHALL create 18 labeled demonstration lots across three events named Analog Memories, Objects of Time, and The Listening Room. It SHALL use all five categories, local media, and valid content/pricing/fulfillment. It SHALL establish dates once from a supplied reference time, remain repeat-safe without overwriting edits, and never run on ordinary startup.

#### Scenario: Seed a demo database
- **WHEN** the seed operation runs against an empty catalog with a reference time
- **THEN** it creates three complete draft events with six distinct lots each and persists their schedules
- **AND** an operator can review and publish an event using the normal validation flow

#### Scenario: Repeat seed or restart
- **WHEN** an operator edits a seed record, reruns the seed operation, or restarts the server
- **THEN** rerunning reports the seed already applied and creates no duplicates
- **AND** neither seed replay nor startup overwrites edits, changes publication state, or rebases stored dates

#### Scenario: Seed failure or identity collision
- **WHEN** seeding fails or a non-seed record already occupies an intended seed identity/slug
- **THEN** seeding leaves no partially created dataset and reports the problem
- **AND** existing user content is preserved

### Requirement: Public catalog response time
Successful public category, event and lot list/detail responses SHALL include `meta.server_time` as integer UTC Unix milliseconds sampled from the server clock for that response. Existing `data` fields, published-only visibility, error envelopes and `Cache-Control: no-store` SHALL remain compatible. Response time SHALL be metadata, separate from immutable editorial records and auction execution state.

#### Scenario: Anonymous list or detail response
- **WHEN** an anonymous client successfully requests any public category, event or lot list/detail endpoint
- **THEN** it receives `{data: ..., meta: {server_time: ...}}` with a valid server instant and the existing public data shape
- **AND** clients that read only `data` continue to work without admin credentials

#### Scenario: Time changes without record mutation
- **WHEN** the same published record is read at two different server instants
- **THEN** its response metadata reflects the respective instants while its stored content, editorial state, prices and planned dates remain unchanged
- **AND** no current bid, winner, execution status or game revision is created or changed by the reads

#### Scenario: Existing privacy and error policy
- **WHEN** public catalog reads encounter a missing/draft slug, storage failure, or records containing private notes
- **THEN** draft and missing slugs retain identical not-found responses, failures retain sanitized error envelopes, and public data exclude private fields and game secrets
- **AND** response time does not authorize private reads or alter the existing admin capability checks

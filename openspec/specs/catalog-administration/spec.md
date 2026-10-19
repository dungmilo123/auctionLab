# catalog-administration Specification

## Purpose

Provide an authorized, accessible preparation workspace for vintage lots and auction events while preserving explicit separation from public previews and future bidder accounts.

## Requirements

### Requirement: Catalog API authorization
Every catalog administration read, preview, and mutation SHALL require the existing valid admin capability cookie. Missing or invalid capabilities SHALL return JSON HTTP 401 with no catalog data, HTML gate, or redirect. Query keys and legacy player tokens SHALL NOT authorize these APIs. Successful responses SHALL NOT contain the capability key, and admin responses SHALL use `Cache-Control: no-store`.

#### Scenario: Anonymous draft access
- **WHEN** a client without a valid capability requests an admin catalog list, draft detail, preview, or mutation
- **THEN** it receives JSON 401 and no private content
- **AND** a correct key supplied only in the API query string or a player token does not grant access

#### Scenario: Authorized operator
- **WHEN** an operator enters the existing document gate and then requests catalog management data with its cookie
- **THEN** the admin API returns the authorized data without exposing the key
- **AND** the `/admin` document gate retains its existing cookie and redirect behavior

### Requirement: Mutation request boundary
Catalog mutations SHALL accept only JSON requests with the custom `X-AuctionLab-Admin: 1` header and a valid capability. A supplied Origin SHALL match the browser-facing request origin; cross-origin requests SHALL be rejected. GET requests SHALL NOT mutate catalog state. Malformed or unsupported requests SHALL return JSON errors without changing data.

#### Scenario: Cross-origin write attempt
- **WHEN** a mutation carries a valid cookie but a foreign Origin, lacks the required header, or uses form content
- **THEN** it is rejected without a catalog change
- **AND** no permissive cross-origin credential authorization is returned

#### Scenario: Valid proxied development request
- **WHEN** the browser sends a same-origin admin mutation through the existing development API proxy with its capability, JSON content, and required header
- **THEN** it receives the same authorization and validation behavior as the embedded Go build
- **AND** no development-only API bypass is used

### Requirement: Lot preparation workspace
`/admin/lots` SHALL provide a stored lot list, draft creation, and editing of catalog content, local media references/order, prices, fulfillment, and private notes. Editors SHALL show draft/published and assignment state. Successful saves SHALL show durable confirmation; loading, empty, validation, authorization, conflict, and server-error states SHALL be explicit. Failed saves SHALL retain unsaved inputs.

#### Scenario: Create and edit a lot
- **WHEN** an authorized operator creates a titled lot and fills its content, gallery, category, prices, and fulfillment fields
- **THEN** saving and reopening displays the persisted values
- **AND** the list distinguishes assigned/unassigned and draft/published records without showing a fabricated live auction status

#### Scenario: Recover from save failure
- **WHEN** a save fails validation, loses authorization, encounters a newer version, or receives a server error
- **THEN** the UI explains the outcome and retains the entered draft values
- **AND** it does not display a successful save or silently retry over a newer record

#### Scenario: PostgreSQL unavailable during editing
- **WHEN** PostgreSQL becomes unavailable while an operator saves a draft or publishes an event
- **THEN** the UI retains unsaved input and reports the failed or uncertain operation without claiming success
- **AND** recovery reloads committed state before retrying an operation whose commit outcome is unknown

### Requirement: Event preparation and publication workspace
`/admin/auctions` SHALL list stored events and support draft creation/editing, cover/content entry, lot selection/removal/order, and schedule configuration. It SHALL display per-lot planned times and an authorized public-field preview before explicit publication. Published events SHALL appear read-only. Incomplete drafts SHALL remain savable, with publication errors attached to the affected event or lot.

#### Scenario: Prepare a complete event
- **WHEN** an operator creates an event, assigns prepared lots, sets the schedule, reviews the public preview, and chooses Publish
- **THEN** the UI displays the committed publication result and read-only event details
- **AND** refreshing shows the same stored data and order

#### Scenario: Correct publication errors
- **WHEN** an operator attempts to publish an event with an incomplete lot
- **THEN** the UI identifies the lot and field to correct while preserving the event draft
- **AND** the operator can open the relevant lot editor, save corrections, and return to publish

### Requirement: Existing routes and direct editor selection
Catalog administration SHALL use the existing `/admin/lots` and `/admin/auctions` pages, with query parameters for new or selected editors. Native document links, direct entry, refresh, and browser navigation SHALL remain usable. Unknown record selections SHALL show an unavailable state within management rather than opening a fabricated editor.

#### Scenario: Deep-link a saved draft
- **WHEN** an authorized operator opens or refreshes `/admin/lots?edit=<id>` or `/admin/auctions?edit=<id>`
- **THEN** the stored selected record appears in its editor
- **AND** page routing still ignores query parameters for layout identity

#### Scenario: Missing editor selection
- **WHEN** a selected ID does not exist
- **THEN** the page shows a recoverable record-not-found state and a link back to its list
- **AND** it does not silently create or display an unrelated record

### Requirement: Accessible bilingual management
Catalog controls SHALL reuse the vintage design system, support English/Vietnamese interface labels and errors, visible keyboard focus, labeled fields, associated help/errors, and usable layouts from 320 CSS pixels through desktop and at 200 percent text zoom. Catalog content SHALL be stored as entered without requiring translations. Times SHALL show a timezone and money SHALL show currency and cents.

#### Scenario: Keyboard and mobile preparation
- **WHEN** an operator edits a long-titled lot and reorders event lots at a narrow viewport using a keyboard
- **THEN** every action remains reachable, ordering has accessible move controls, and fields do not cause page-level horizontal overflow
- **AND** errors identify the affected controls in the selected interface language

#### Scenario: Switch interface language
- **WHEN** an operator switches between English and Vietnamese
- **THEN** management labels, state messages, validation feedback, and document language reflect that choice
- **AND** entered catalog text retains its original content

### Requirement: Honest management and development boundaries
Catalog pages SHALL identify working content management and available commerce accounts/watchlists/scheduled bidding while checkout/orders remain unavailable. Bid participation SHALL show confirmed owner data. Publication SHALL remain a preparation action; the separate lifecycle executes its immutable schedules. A development admin page without a capability SHALL show an access-required state linking to the existing Go gate, preserve the HttpOnly credential boundary, and never expose draft data or simulate a successful save.

#### Scenario: Frontend-only development
- **WHEN** an operator opens a catalog admin page on the development frontend without a reachable authorized API
- **THEN** the page explains that access or the application server is required
- **AND** it does not fall back to mutable sample data or request a secret through a frontend field

#### Scenario: Return to public previews
- **WHEN** an operator saves or publishes content and visits the storefront, account, or order pages
- **THEN** discovery remains public, account/profile/watchlist pages use their independent commerce session, and bid participation and lot execution use the separate commerce service while checkout/order activity remains explicitly unavailable
- **AND** catalog management itself neither signs an account in nor creates watch entries, bids, checkout or orders; publication makes its scheduled lots eligible for independent execution

#### Scenario: Account sessions do not authorize catalog management
- **WHEN** a bidder or admin account session is present without a valid catalog capability
- **THEN** management documents and every private catalog read, preview, mutation, and unknown private path retain the existing gate/JSON-401 behavior
- **AND** acquiring the catalog capability does not reveal or select another person's commerce account

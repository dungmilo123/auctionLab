# Spec Delta

## Purpose

Let each buyer revisit their own purchases and follow committed fulfillment progress without exposing another buyer's purchase or contact information.

## ADDED Requirements

### Requirement: Owned order history collection
`GET /api/account/orders` SHALL return every order owned by the current commerce session, including unpaid, paid and completed purchases, ordered by result-recording time descending then order ID ascending. Summaries SHALL omit contact, address and receipt details. Reads SHALL be no-store and SHALL NOT create or change purchases.

#### Scenario: History includes every purchase stage
- **WHEN** an owner has unpaid, shipping-in-progress and completed pickup orders
- **THEN** history returns all three once in deterministic order with snapshot title/image, currency, winning amount, payment state and fulfillment status
- **AND** unpaid orders show fulfillment method and final total as unselected rather than inventing either

#### Scenario: Empty and failed history
- **WHEN** an authenticated owner has no orders or storage cannot answer the read
- **THEN** the former returns HTTP 200 with an empty collection and the latter a sanitized service failure
- **AND** failure is not rendered as an empty purchase history

### Requirement: Owner-only history access
History and owner detail SHALL derive account identity from the current durable session. Missing, expired or revoked sessions SHALL receive HTTP 401; foreign and missing order IDs SHALL share HTTP 404. Account admin roles and the capability cookie SHALL NOT broaden access in the account namespace.

#### Scenario: Two buyers and an operator
- **WHEN** two buyers list/read orders and a capability-only operator requests `/api/account/orders` or an owner order URL
- **THEN** each buyer sees only their own orders and the operator receives 401 without an owner session
- **AND** a logged-in commerce admin receives 404 for another owner's order

### Requirement: Stored purchase detail and timeline
`/account/orders/:orderId` SHALL show the owned immutable snapshot, payment state, selected fulfillment/contact and exact totals/receipt when paid, plus committed fulfillment status, timeline and shipping information. Unpaid detail SHALL link to existing checkout; paid detail SHALL offer no new payment action. All payments and fulfillment SHALL be labeled simulated.

#### Scenario: Buyer follows shipping progress
- **WHEN** an operator commits preparing, shipped and completed transitions and the buyer refreshes the same order
- **THEN** detail shows those committed events, server times with timezone and manually supplied carrier/reference at shipment
- **AND** the original snapshot, selection, amounts and receipt remain identical

#### Scenario: Historic adopted purchase
- **WHEN** an order was created after its original auction result and later paid
- **THEN** the timeline distinguishes original result time, order creation time and stored payment time
- **AND** upgrade does not invent preparation, shipment, collection or completion events

### Requirement: Accessible private history navigation
History/detail SHALL support EN/VI, keyboard navigation, 320/375 CSS-pixel and desktop layouts, actual 200% browser zoom and reduced motion. Loading, signed-out, empty, missing and unavailable states SHALL differ. Logout, account switch and restored pages SHALL clear private data and suppress obsolete responses.

#### Scenario: Detail deep link and identity change
- **WHEN** a buyer opens detail directly, uses Back/Forward, changes language, logs out or switches accounts while a read is pending
- **THEN** the page validates the current session, preserves safe navigation and shows only the current owner's confirmed data
- **AND** stale responses cannot restore the previous owner's order, address or receipt

#### Scenario: Narrow layout and lengthy content
- **WHEN** long titles, carrier/reference values and timeline text are used in EN/VI at narrow widths and actual 200% browser zoom
- **THEN** content and recovery controls remain readable and keyboard reachable without page-level horizontal overflow
- **AND** state is expressed in text and nonessential motion respects the active reduced-motion preference

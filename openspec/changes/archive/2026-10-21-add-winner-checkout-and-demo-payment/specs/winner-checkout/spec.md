# Spec Delta

## Purpose

Turn durable sold auction results into private, single-lot purchase obligations that winners can review and complete using the published fulfillment terms.

## ADDED Requirements

### Requirement: Exactly one order for each sold result
The system SHALL automatically create one awaiting-payment order owned by the winning account for each sold commerce result. New sold closures SHALL commit result and order together. Unsold, open, scheduled, draft and missing lots SHALL create no order. Order creation SHALL NOT require a browser visit, change the winner or charge money.

#### Scenario: Sold and unsold closures
- **WHEN** two lots close, one with a highest accepted bid and one without bids
- **THEN** the sold result has exactly one order for its winning account and exact winning bid/amount, while the unsold lot has none
- **AND** repeating closure cannot create another order or change the result

#### Scenario: Closure transaction fails
- **WHEN** recording either the new sold result or its order fails before commit
- **THEN** neither is committed and the lot remains eligible for closure reconciliation
- **AND** no success is announced from partial state

### Requirement: Adopt previously recorded winners
Normal startup SHALL create missing orders for previously committed sold results before serving traffic and SHALL refuse readiness if this reconciliation fails. Adoption SHALL preserve original result data, distinguish historical close/result times from order creation time, and remain repeat-safe. Offline migration/import/seed modes SHALL NOT perform adoption or create payments.

#### Scenario: Upgrade with historic sold results
- **WHEN** an existing database contains sold and unsold results with no orders
- **THEN** normal startup creates one unpaid order per sold result and none for unsold results without inventing bids or payments
- **AND** restart preserves existing order IDs, snapshots and payment state

### Requirement: Immutable purchase snapshot
Each order SHALL retain lot/event IDs, public lot slug/title/condition/primary image with alternative text, winning bid/account/amount, USD currency, close/result times, demo provenance and published fulfillment methods, instructions and shipping fee. These purchase terms SHALL be immutable; later profile changes SHALL NOT rewrite submitted checkout contact details.

#### Scenario: Review survives restart
- **WHEN** the owner reopens an order after restart or changes their profile display name
- **THEN** its purchase terms and previously submitted checkout contact details remain unchanged
- **AND** the snapshot contains no catalog admin notes, capability keys, game secrets or other participants' identity data

### Requirement: Private unpaid wins and order reads
`/api/account/wins` SHALL list only the session owner's awaiting-payment orders, newest result-recording time first then order ID. `/api/account/orders/:orderId` SHALL return only that owner's checkout/confirmation data. Missing/foreign IDs SHALL share HTTP 404; invalid sessions SHALL receive HTTP 401. All private responses SHALL be no-store, and reads SHALL NOT create orders or payments.

#### Scenario: Winner, loser and capability holder
- **WHEN** the winner, a losing bidder, or a client with only an admin capability requests the order
- **THEN** only the winner's valid commerce session returns it; the losing bidder receives 404 and the capability-only client receives 401
- **AND** commerce admin accounts can access only orders they themselves won

#### Scenario: Unpaid list after payment
- **WHEN** an owned order becomes paid
- **THEN** it disappears from unpaid wins and its direct private URL still returns the same paid confirmation
- **AND** an owner without unpaid wins receives a successful empty list, distinct from backend failure

### Requirement: Single-order fulfillment and contact
Checkout SHALL accept contact name, email and phone plus one fulfillment option enabled in the order snapshot. Delivery SHALL require address line 1, locality and two-letter country code, with optional line 2, region and postal code. Pickup SHALL require no address and SHALL discard address fields. Invalid or oversized values SHALL produce field errors without changing the order.

#### Scenario: Delivery and pickup
- **WHEN** the winner chooses delivery with complete contact/address or pickup with complete contact
- **THEN** checkout can review that enabled method with its published instructions
- **AND** pickup retains no submitted shipping address

#### Scenario: Invalid checkout details
- **WHEN** a required field is blank, email/country syntax is invalid, a string exceeds its documented limit or an unavailable method is submitted
- **THEN** the response identifies the invalid fields and records no payment or paid transition
- **AND** valid entered values remain available in the active form for correction

### Requirement: Exact server-calculated checkout total
The server SHALL calculate `total_minor = winning_amount_minor + fulfillment_fee_minor` using integer USD cents from the immutable order snapshot. Delivery SHALL use its published flat fee; pickup SHALL use zero. Winning amount and fee SHALL each support 0 through 1,000,000,000,000 cents; their total SHALL support 0 through 2,000,000,000,000 cents. No other charge SHALL be added.

#### Scenario: Published delivery charge
- **WHEN** the winning amount is 12550 cents and published shipping is 1500 cents
- **THEN** the delivery review and confirmation show USD 125.50 plus USD 15.00, total USD 140.50
- **AND** selecting available pickup shows a zero fee and total USD 125.50

#### Scenario: Zero and upper bounds
- **WHEN** a winning amount or shipping fee is zero, or both components equal their maximum
- **THEN** zero remains a valid explicit amount and the maximum delivery total is exactly 2,000,000,000,000 cents without overflow or rounding
- **AND** absent or corrupt required terms produce unavailable checkout rather than a fabricated zero

#### Scenario: Client attempts to choose a price
- **WHEN** a caller supplies amount, currency, fee, total, winner or owner fields in a checkout request
- **THEN** unsupported fields are rejected and no order/payment change occurs
- **AND** successful reviews and confirmations always use the server-calculated values

### Requirement: Stateless server checkout review
`POST /api/account/orders/:orderId/review` SHALL validate owner access and normalized contact/fulfillment/simulation choice, then return the exact server-calculated purchase review without persisting changes. It SHALL use the same strict account request boundary as confirmation. A paid order SHALL return its existing confirmation rather than a new payable review.

#### Scenario: Review before confirmation
- **WHEN** an owner reviews complete valid checkout fields on an unpaid order
- **THEN** the response shows normalized contact, selected published instructions and exact item/fee/total values
- **AND** the order remains awaiting payment without saved contact/address or a payment receipt until explicit confirmation

#### Scenario: Review failure or paid order
- **WHEN** review has invalid fields, unavailable ownership, or an already-paid order
- **THEN** it returns field/access recovery or the existing paid confirmation as appropriate
- **AND** no order data is changed or second payment offered

### Requirement: Accessible owned checkout experience
`/account/wins` and `/account/checkout/:orderId` SHALL provide EN/VI labels and states, exact currency displays, labeled keyboard controls and usable mobile/desktop layouts at actual 200-percent zoom. Loading, empty, signed-out, missing, unavailable, validation, pending, unknown and paid states SHALL be distinct. Logout/account switch SHALL clear private data and contact/address values; stale responses SHALL NOT restore them.

#### Scenario: Owner completes the purchase review
- **WHEN** a winner follows a won-lot link, selects fulfillment and a simulation method, and reviews the order
- **THEN** the lot snapshot, contact, fulfillment instructions, item amount, fee, total and simulation notice are visible before deliberate confirmation
- **AND** refresh retrieves committed order state without automatically submitting payment or persisting unsent contact/address fields

#### Scenario: Mobile, language and identity recovery
- **WHEN** checkout is used at 320/375 CSS pixels, desktop or actual 200-percent browser zoom, in EN/VI with keyboard navigation, or its session expires
- **THEN** controls/errors remain reachable without page-level horizontal overflow and the page offers safe login return when needed
- **AND** obsolete reads or confirmation responses cannot reveal the previous account's order or restore cleared form values

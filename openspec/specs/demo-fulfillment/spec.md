# demo-fulfillment Specification

## Purpose

Give authorized operators and buyers a shared, durable record of simulated delivery or collection after an auction purchase has been paid.

## Requirements

### Requirement: Dedicated operator order access
Every `/api/admin/orders` read, detail, transition and unknown subpath SHALL independently require the existing admin capability cookie. Missing/invalid capability SHALL return no-store JSON 401 without data or redirects. Account roles, game tokens and query keys SHALL NOT authorize these APIs. Operator access SHALL NOT impersonate a buyer or authorize payment.

#### Scenario: Operator and account boundaries
- **WHEN** a capability holder requests the order workspace and its API without a commerce session
- **THEN** dedicated admin reads succeed while account order/checkout APIs still require their owner session
- **AND** an admin account without the capability cannot list or update others' orders

### Requirement: Filterable operator order workspace
`/admin/orders` SHALL list stored orders newest result-recording time first then ID, filter by fulfillment status and selected method, and inspect a selected order through `?view=<orderId>`. List summaries SHALL omit buyer contact/address and receipt details. Authorized detail SHALL expose only purchase, fulfillment contact/address, simulation receipt and timeline data needed to process that order.

#### Scenario: Filter and inspect
- **WHEN** an operator selects status `preparing` and method `shipping`, then opens an order and refreshes
- **THEN** the API returns only matching orders and the selected stored detail remains reachable
- **AND** missing selection, invalid filters, empty matches, lost access and service failure have explicit recovery states

### Requirement: Method-specific forward progression
Shipping SHALL follow `awaiting_payment → paid → preparing → shipped → completed`; pickup SHALL follow `awaiting_payment → paid → ready_for_pickup → completed`. Only owner payment confirmation SHALL perform the unpaid-to-paid step. Operators SHALL advance exactly one allowed step using the immutable paid method. Completion SHALL be terminal.

#### Scenario: Shipping and pickup complete
- **WHEN** paid shipping and pickup orders advance through their respective paths
- **THEN** operator and owner reads show the same committed progress and stored simulated payment
- **AND** neither path changes method, contact, price, receipt or auction result

#### Scenario: Invalid progression
- **WHEN** an operator attempts fulfillment before payment, a skipped/backward step, a method-incompatible step or reopening completion
- **THEN** the request returns conflict and no progress/history changes
- **AND** operator controls offer only the next valid step and never a pay-on-behalf action

### Requirement: Manual simulated shipment information
The shipping transition to `shipped` SHALL require carrier and tracking-reference text, trimmed and control-free, limited to 120 and 200 Unicode code points respectively. These values SHALL commit with shipment, remain immutable and display as text to the owner/operator using ordinary shipment labels without required simulated/manual badges. Pickup SHALL reject shipping fields. No external tracking request or automatic delivery claim SHALL occur.

#### Scenario: Record shipment
- **WHEN** an operator submits valid carrier/reference while advancing preparing to shipped
- **THEN** the values appear with the committed shipment event and natural shipment labels without a simulation disclaimer
- **AND** text resembling markup or a URL is safely rendered without executable markup or an automatic external lookup

#### Scenario: Invalid shipping data
- **WHEN** shipment lacks a required value, exceeds a limit, contains controls, or shipping fields accompany another transition
- **THEN** field validation fails without changing state or timeline
- **AND** valid entered values remain available for correction while access remains valid

### Requirement: Atomic versioned fulfillment transitions
Transitions SHALL require the last-read fulfillment version and atomically store one new version, next status, server timestamp and durable timeline event. Conflicting concurrent/stale requests SHALL return 409. A repeated stale request SHALL NOT append another event or advance again. Pre-commit failure SHALL leave all progress unchanged; success SHALL require confirmed commit.

#### Scenario: Concurrent or duplicate operators
- **WHEN** two requests submit the same expected version, or a successful request is repeated after restart
- **THEN** at most one advances the order once and the other returns a conflict requiring a fresh read
- **AND** no duplicate timeline event or silent overwrite occurs

#### Scenario: Unknown commit outcome
- **WHEN** a transition response is lost or commit cannot be established
- **THEN** the workspace shows an unknown outcome and offers a durable detail reload before any further deliberate action
- **AND** it neither announces success nor automatically retries with a newer version

### Requirement: Protected transition request boundary
Operator transitions SHALL require same-origin protection, `X-AuctionLab-Admin: 1`, bounded strict JSON and the capability. Unsupported fields, methods and bodies SHALL be rejected without mutation. Responses/errors SHALL be no-store and sanitized; contact/address, credentials and request bodies SHALL stay out of logs, URLs and browser persistence.

#### Scenario: Forged or oversized update
- **WHEN** a transition has a foreign Origin, absent header, non-JSON or over-8-KiB body, or supplied owner/amount/contact fields
- **THEN** it fails at the request boundary without a state change or echoed submitted data
- **AND** Vite-proxied and embedded requests follow the same checks

### Requirement: Durable ordered fulfillment timeline
Owner and operator detail SHALL show order creation, stored payment and every accepted fulfillment event using durable server timestamps and sequence order. Upgrade SHALL initialize paid progress from original payment time without inventing later events. Events SHALL identify payment/system versus operator action without attributing a shared capability to a named account.

#### Scenario: Refresh and restart
- **WHEN** paid and in-progress orders are reloaded after application/PostgreSQL restart
- **THEN** their status, shipping data and timeline remain identical in owner/operator reads
- **AND** equal timestamps retain deterministic sequence order and later events do not precede their predecessor's effective time

### Requirement: Accessible operator fulfillment controls
The workspace SHALL provide EN/VI, labeled keyboard controls, explicit pending/conflict/unknown/error states and usable narrow/desktop/actual-200%-zoom layouts with reduced motion. Navigation, access loss and page restoration SHALL clear sensitive cached detail and suppress late responses. Language changes SHALL retain authorized unsaved shipping text. The workspace SHALL group filters, summaries, selected-order details, timeline and next valid action with coherent iconography, natural action labels and visible confirmation text.

#### Scenario: Recover without private stale data
- **WHEN** capability access expires during a read or update, or the operator changes order selection while a response is pending
- **THEN** the workspace clears obsolete contact/detail immediately and shows access-required or the current selected-order recovery state
- **AND** returning through the Go gate never replays the previous transition automatically

#### Scenario: Fulfillment review and confirmation
- **WHEN** an operator reviews shipping values or advances an order
- **THEN** the current selected order, immutable purchase terms and next permitted transition remain clear, with pending controls preventing competing submissions
- **AND** labels omit simulation qualifiers while version checks, durable reload after conflict/unknown and one-step progression remain unchanged

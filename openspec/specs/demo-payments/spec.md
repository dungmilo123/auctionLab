# demo-payments Specification

## Purpose

Let auction winners complete an explicitly simulated payment and retrieve a durable confirmation without collecting financial credentials or moving real money.

## Requirements

### Requirement: Explicit simulated payment methods
Checkout SHALL accept only internal methods `demo-card` and `demo-bank-transfer`, displayed to buyers as localized Card and Bank transfer choices with natural confirmation/receipt labels. Buyer UI SHALL omit demo/simulation and no-charge disclaimers. Processing SHALL remain local simulation. No card number, expiry, CVV, bank-account number or real payment credential SHALL be requested, accepted or stored; there SHALL be no gateway, bank redirect or external payment request.

#### Scenario: Select either simulation method
- **WHEN** the winner selects either allowed method
- **THEN** the review identifies the selected localized method without showing internal identifiers or real financial-credential fields
- **AND** confirmation creates only the existing local simulated receipt, never an actual charge, and the UI adds no simulation or no-charge banner

#### Scenario: Unsupported method or financial fields
- **WHEN** a request specifies another method or supplies card/bank fields
- **THEN** it is rejected without a payment or order transition
- **AND** the response does not echo submitted financial values

### Requirement: Owner-authorized confirmation request
`POST /api/account/orders/:orderId/confirm` SHALL require a current owner commerce session, bounded strict JSON, `X-AuctionLab-Account: 1` and same-origin request protection. The server SHALL derive ownership and price from durable records and recheck session validity after mutation/row-lock waiting. Admin capability and game tokens SHALL NOT authorize confirmation.

#### Scenario: Authorization or boundary failure
- **WHEN** confirmation has an absent/expired/revoked session, a foreign order, a foreign Origin, missing header, unsupported JSON fields or an oversized body
- **THEN** it is refused with a structured no-store response and no payment change
- **AND** a foreign order is indistinguishable from a missing order for an authenticated caller

#### Scenario: Session expires while waiting
- **WHEN** a valid request waits for mutation ownership until its session has expired
- **THEN** confirmation returns HTTP 401 and records no payment
- **AND** login recovery requires another deliberate confirmation

### Requirement: Atomic one-payment transition
Successful confirmation SHALL atomically store the validated contact/fulfillment selection, exact server-calculated amounts, one immutable simulated payment receipt, the order's paid state/time and initial paid fulfillment progress. Each order SHALL have at most one payment. Failed validation or pre-commit storage failure SHALL leave it awaiting payment without partial contact/payment/fulfillment state. Success SHALL require confirmed commit.

#### Scenario: Confirmation and restart
- **WHEN** an owner confirms complete valid checkout details
- **THEN** one receipt identifies the same order, simulation method, USD total and paid time and the order is paid
- **AND** restart and direct private reads return that same confirmation with no second payable balance

#### Scenario: Concurrent confirmations
- **WHEN** two requests with different request IDs attempt to pay one order concurrently
- **THEN** at most one creates the receipt and paid transition; the other returns the existing confirmation with an already-paid indication
- **AND** the later request cannot change stored contact, fulfillment, method, amount or paid time

#### Scenario: Fulfillment initialization fails
- **WHEN** storing initial paid fulfillment progress fails before the confirmation commits
- **THEN** the order remains awaiting payment without a receipt or partial fulfillment record
- **AND** a replay of an already accepted payment never resets later fulfillment progress

### Requirement: Retry-safe confirmation intents
Confirmation SHALL carry a UUID request ID scoped to the owner account. An accepted ID replayed with the same normalized order/contact/fulfillment/method SHALL return its original receipt; a different payload SHALL conflict. Validation failures SHALL NOT reserve an ID. Fresh requests for an already-paid order SHALL return its existing receipt without recording another payment or changing checkout data.

#### Scenario: Duplicate submit and altered replay
- **WHEN** the same accepted intent is submitted twice, retried after restart, or replayed with a different method/address/order
- **THEN** matching replays identify the original receipt and changed replays return a request conflict
- **AND** there is still exactly one payment for the order

#### Scenario: Retry after correction
- **WHEN** an invalid confirmation is corrected before any payment commits
- **THEN** the corrected request can succeed without the failed validation claiming its request ID
- **AND** an account-scoped accepted ID cannot be reused to pay a different order

### Requirement: Recover uncertain confirmation from durable state
A timeout, transport failure or unknown commit SHALL produce a pending/unknown outcome rather than success or definitive unpaid status. The website SHALL reconcile through an authenticated order read or deliberate retry of the same intent with the same ID. It SHALL NOT automatically generate another intent, replay after login or announce payment from local state alone.

#### Scenario: Response is lost after commit
- **WHEN** confirmation commits but its response is lost
- **THEN** an owner read or same-intent retry retrieves the single paid confirmation
- **AND** the website offers recovery without asking for a second payment or changing the submitted selection

#### Scenario: Commit cannot be established
- **WHEN** neither confirmation nor its recovery read can establish durable payment state
- **THEN** the page keeps an explicit unknown/unavailable outcome with recovery controls
- **AND** it does not display paid success or create a fresh confirmation intent automatically

### Requirement: Durable private simulation confirmation
The owner checkout URL SHALL display paid confirmation from stored data, including receipt ID, order ID, snapshot lot, selected fulfillment and instructions, contact, item/fee/total, currency, selected method and paid time. The page SHALL use natural product labels without simulation/no-charge notices, remain read-only after payment and survive refresh. Receipt/contact data SHALL NOT appear in public catalog, auction history, streams or another account's owner-namespace responses. Dedicated capability-authorized admin order detail SHALL expose the bounded operational purchase projection without granting payment or account impersonation.

#### Scenario: Reopen confirmation safely
- **WHEN** the owner refreshes the paid checkout URL or follows its won-participation link later
- **THEN** the server-confirmed receipt and purchase details appear with no actionable pay control, demo badge or simulation/no-charge notice
- **AND** logout or account switching clears the previous owner's details and suppresses late responses

#### Scenario: Operator inspects a paid order
- **WHEN** an authorized capability holder reads dedicated admin order detail
- **THEN** it can inspect the stored local receipt and fulfillment contact needed to process that order
- **AND** public feeds and foreign account reads still reveal no receipt/contact data, and operator documentation retains the simulated processing definition

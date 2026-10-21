# Spec Delta

## MODIFIED Requirements

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

### Requirement: Durable private simulation confirmation
The owner checkout URL SHALL display paid confirmation from stored data, including receipt ID, order ID, snapshot lot, selected fulfillment and instructions, contact, item/fee/total, currency, simulation method and paid time. The page SHALL retain the simulation label, remain read-only after payment and survive refresh. Receipt/contact data SHALL NOT appear in public catalog, auction history, streams or another account's owner-namespace responses. Dedicated capability-authorized admin order detail SHALL expose the bounded operational purchase projection without granting payment or account impersonation.

#### Scenario: Reopen confirmation safely
- **WHEN** the owner refreshes the paid checkout URL or follows its won-participation link later
- **THEN** the server-confirmed receipt and purchase details appear with no actionable pay control
- **AND** logout or account switching clears the previous owner's details and suppresses late responses

#### Scenario: Operator inspects a paid order
- **WHEN** an authorized capability holder reads dedicated admin order detail
- **THEN** it can inspect the stored simulated receipt and fulfillment contact needed to process that order
- **AND** public feeds and foreign account reads still reveal no receipt/contact data

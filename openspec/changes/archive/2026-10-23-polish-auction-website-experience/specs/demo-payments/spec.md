# Spec Delta

## MODIFIED Requirements

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

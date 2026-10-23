# Spec Delta

## MODIFIED Requirements

### Requirement: Accessible owned checkout experience
`/account/wins` and `/account/checkout/:orderId` SHALL provide EN/VI labels and states, exact currency displays, labeled keyboard controls and usable mobile/desktop layouts at actual 200-percent zoom. Loading, empty, signed-out, missing, unavailable, validation, pending, unknown and paid states SHALL be distinct. Logout/account switch SHALL clear private data and contact/address values; stale responses SHALL NOT restore them. Checkout SHALL group contact, enabled fulfillment and payment selection, keep a legible order summary and show form, review and confirmation as clear steps without demo/simulation/no-charge notices.

#### Scenario: Owner completes the purchase review
- **WHEN** a winner follows a won-lot link, selects fulfillment and an allowed payment method, and reviews the order
- **THEN** the lot snapshot, contact, fulfillment instructions, exact item amount, fee, total and selected method are visible before deliberate confirmation with natural product labels
- **AND** refresh retrieves committed order state without automatically submitting payment or persisting unsent contact/address fields

#### Scenario: Mobile, language and identity recovery
- **WHEN** checkout is used at 320/375 CSS pixels, desktop or actual 200-percent browser zoom, in EN/VI with keyboard navigation, or its session expires
- **THEN** controls/errors remain reachable without page-level horizontal overflow and the page offers safe login return when needed
- **AND** obsolete reads or confirmation responses cannot reveal the previous account's order or restore cleared form values

#### Scenario: Edit a reviewed purchase
- **WHEN** the owner changes contact, fulfillment or payment method after review
- **THEN** the old review and confirmation intent are invalidated and valid values remain available for a new deliberate review
- **AND** visual step transitions neither submit a payment nor retain another account's order or form values

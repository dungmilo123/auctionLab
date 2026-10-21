# Spec Delta

## MODIFIED Requirements

### Requirement: Private unpaid wins and order reads
`/api/account/wins` SHALL list only the session owner's awaiting-payment orders, newest result-recording time first then order ID. `/api/account/orders/:orderId` SHALL return only that owner's checkout/confirmation data. Missing/foreign IDs SHALL share HTTP 404; invalid sessions SHALL receive HTTP 401. All private responses SHALL be no-store, and reads SHALL NOT create orders or payments. These account routes SHALL remain owner-only; dedicated capability-authorized admin order APIs are a separate operational projection.

#### Scenario: Winner, loser and capability holder
- **WHEN** the winner, a losing bidder, or a client with only an admin capability requests the order
- **THEN** only the winner's valid commerce session returns it; the losing bidder receives 404 and the capability-only client receives 401
- **AND** commerce admin accounts can access only orders they themselves won

#### Scenario: Unpaid list after payment
- **WHEN** an owned order becomes paid
- **THEN** it disappears from unpaid wins and its direct private URL still returns the same paid confirmation
- **AND** an owner without unpaid wins receives a successful empty list, distinct from backend failure

#### Scenario: Fulfillment after checkout
- **WHEN** an owner reopens a paid checkout after fulfillment has progressed
- **THEN** the original paid confirmation remains read-only and links to `/account/orders/<orderId>` for its timeline
- **AND** history detail links back to the same checkout for unpaid purchases and never offers another payment on paid orders

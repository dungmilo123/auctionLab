# Spec Delta

## MODIFIED Requirements

### Requirement: Stored purchase detail and timeline
`/account/orders/:orderId` SHALL show the owned immutable snapshot, payment state, selected fulfillment/contact and exact totals/receipt when paid, plus committed fulfillment status, timeline and shipping information. Unpaid detail SHALL link to existing checkout; paid detail SHALL offer no new payment action. Summaries and detail SHALL use natural payment/fulfillment labels without demo/simulation badges or no-charge disclaimers. Timeline icons and visual progress SHALL correspond only to committed events; future steps SHALL remain clearly pending.

#### Scenario: Buyer follows shipping progress
- **WHEN** an operator commits preparing, shipped and completed transitions and the buyer refreshes the same order
- **THEN** detail shows those committed events, server times with timezone and manually supplied carrier/reference at shipment
- **AND** the original snapshot, selection, amounts and receipt remain identical and carrier/reference remain inert text rather than an automatic tracking-service claim

#### Scenario: Historic adopted purchase
- **WHEN** an order was created after its original auction result and later paid
- **THEN** the timeline distinguishes original result time, order creation time and stored payment time
- **AND** upgrade does not invent preparation, shipment, collection or completion events

#### Scenario: Compact purchase summary
- **WHEN** an owner views history with long titles and orders at different stages
- **THEN** each summary emphasizes its image/title, amount, textual state and detail action, omitting contact/address and receipt details
- **AND** a progress treatment never presents an unpaid purchase as paid or a future fulfillment step as completed

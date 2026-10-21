# Spec Delta

## MODIFIED Requirements

### Requirement: Price and fulfillment disclosure
Lot detail SHALL label the stored USD starting price as an opening price and show its fixed increment. It SHALL disclose condition and every enabled delivery/pickup method, instructions, and shipping fee before the bid control in reading order. Missing amounts SHALL be unavailable, distinct from zero. Current bids, counts, history and results SHALL appear only from the commerce auction service; purchase totals SHALL be available only from owned checkout after a sold result, not invented by public discovery.

#### Scenario: Zero price and free shipping
- **WHEN** a published lot has zero opening cents and zero shipping cents
- **THEN** the page displays both as explicit zero USD amounts rather than unknown values
- **AND** it displays the positive increment, delivery instructions and any enabled pickup instructions

#### Scenario: Pickup only
- **WHEN** a published lot enables only pickup
- **THEN** pickup instructions and condition appear before the bid control and shipping is identified as unavailable
- **AND** no shipping charge or purchase total is invented; bidding follows the separate commerce eligibility rules

### Requirement: Accessible bilingual discovery without transactions
Discovery SHALL follow the existing design system at mobile and desktop widths, support keyboard use, 200 percent text zoom and reduced motion, and supply EN/VI interface, state and recovery copy. Published editorial content SHALL remain as authored. Demo records SHALL retain a visible demo label. Lot-detail bidding SHALL require a current commerce session and confirmed eligibility; anonymous discovery/history SHALL require no account or game session. Winner checkout SHALL link through owner-authenticated order reads; order history and fulfillment SHALL remain unavailable previews.

#### Scenario: Mobile and language switch
- **WHEN** a visitor browses at 320 or 375 CSS pixels, changes EN/VI, or enlarges text to 200 percent
- **THEN** navigation, query controls, gallery, prices and fulfillment remain readable and usable without page-level horizontal overflow
- **AND** language changes preserve loaded records and selections, translate interface labels, and update the document language

#### Scenario: Real record versus demo and future bidding
- **WHEN** a visitor views a stored demo or non-demo lot and attempts the bid control
- **THEN** only demos are labeled as demonstration content, and the bid panel reports the server-confirmed execution state, offering login or deliberate submission only when eligible
- **AND** browsing alone creates no account, watchlist, bid, order or payment mutation, restores no game token and opens no game WebSocket; only deliberate eligible bidding can create a commerce bid and only server closure can create its winner order

#### Scenario: Winner continues from the public lot
- **WHEN** an authenticated winner views a sold lot with confirmed owned participation
- **THEN** a link leads to that owner's checkout or paid confirmation without placing purchase details into the public lot response
- **AND** anonymous/losing visitors see only the public result and public discovery remains usable if checkout is unavailable

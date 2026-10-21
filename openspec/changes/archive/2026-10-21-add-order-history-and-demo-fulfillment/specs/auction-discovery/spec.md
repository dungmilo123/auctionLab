# Spec Delta

## MODIFIED Requirements

### Requirement: Accessible bilingual discovery without transactions
Discovery SHALL follow the existing design system at mobile and desktop widths, support keyboard use, 200 percent text zoom and reduced motion, and supply EN/VI interface, state and recovery copy. Published editorial content SHALL remain as authored. Demo records SHALL retain a visible demo label. Lot-detail bidding SHALL require a current commerce session and confirmed eligibility; anonymous discovery/history SHALL require no account or game session. Winner checkout, order history and fulfillment detail SHALL link through owner-authenticated reads; public discovery SHALL expose no private purchase or shipping data. Notices SHALL identify payment and fulfillment as simulated and keep analytics/recommendations as future work.

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
- **THEN** links lead to that owner's checkout/confirmation and order history/detail without placing purchase or fulfillment details into the public lot response
- **AND** anonymous/losing visitors see only the public result and public discovery remains usable if checkout is unavailable

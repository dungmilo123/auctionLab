# Spec Delta

## MODIFIED Requirements

### Requirement: Accessible bilingual discovery without transactions
Discovery SHALL follow the existing design system at mobile and desktop widths, support keyboard use, 200 percent text zoom and reduced motion, and supply EN/VI interface, state and recovery copy. Published editorial content SHALL remain as authored. Demo records SHALL retain a visible demo label. Lot-detail bidding SHALL require a current commerce session and confirmed eligibility; anonymous discovery/history SHALL require no account or game session. Winner checkout, order history and fulfillment detail SHALL link through owner-authenticated reads; public discovery SHALL expose no private purchase or shipping data. Notices SHALL identify payment and fulfillment as simulated, describe available rule-based recommendations and keep trained ML as separate future work. Optional first-party view recording SHALL use the separate interaction endpoint without blocking browsing or changing commerce state.

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

## ADDED Requirements

### Requirement: Independent recommendation discovery
Home and lot detail SHALL load rule-based suggestions independently from existing catalog and auction execution. Existing homepage featured events, nearest-close sections, query semantics and opening-price sorts SHALL remain intact. Suggestions SHALL use only safe published cards and native stored-slug links.

#### Scenario: Suggestions and observations do not transact
- **WHEN** a visitor loads a published lot or follows a recommendation
- **THEN** related-lot loading and optional view recording create no account, watch entry, bid, result, order or payment
- **AND** a failed recommendation or observation request leaves the loaded gallery, fulfillment terms and eligible deliberate bid controls usable

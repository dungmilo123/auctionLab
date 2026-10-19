# Spec Delta

## MODIFIED Requirements

### Requirement: Price and fulfillment disclosure
Lot detail SHALL label the stored USD starting price as an opening price and show its fixed increment. It SHALL disclose condition and every enabled delivery/pickup method, instructions, and shipping fee before the bid control in reading order. Missing amounts SHALL be unavailable, distinct from zero. Current bids, counts, history and results SHALL appear only from the commerce auction service; purchase totals SHALL remain unavailable.

#### Scenario: Zero price and free shipping
- **WHEN** a published lot has zero opening cents and zero shipping cents
- **THEN** the page displays both as explicit zero USD amounts rather than unknown values
- **AND** it displays the positive increment, delivery instructions and any enabled pickup instructions

#### Scenario: Pickup only
- **WHEN** a published lot enables only pickup
- **THEN** pickup instructions and condition appear before the bid control and shipping is identified as unavailable
- **AND** no shipping charge or purchase total is invented; bidding follows the separate commerce eligibility rules

### Requirement: Honest planned schedule phases
Discovery SHALL classify a record as upcoming when time is before opening, within planned window from opening inclusive until closing exclusive, and elapsed at or after closing. Events SHALL use their planned end and lots their own close. Phase labels SHALL explicitly describe schedules. A separate lot-detail execution panel SHALL use only server-confirmed auction snapshots for live or terminal claims. Invalid or missing timing SHALL display schedule unavailable.

#### Scenario: Exact schedule boundaries
- **WHEN** the server-referenced presentation clock reaches opening or closing
- **THEN** the phase changes to within planned window at opening and elapsed at closing
- **AND** displayed dates identify the viewer's timezone, negative countdowns are avoided, and no catalog or game state is mutated

#### Scenario: Clock and elapsed schedule
- **WHEN** a device clock is incorrect or a visitor loads a published record after its planned end
- **THEN** phase selection uses successful public-response server time and elapsed time since receipt rather than the device wall clock
- **AND** schedule classification never determines an outcome; the separate execution panel can show only a server-confirmed result or an honest pending/unavailable state

### Requirement: Accessible bilingual discovery without transactions
Discovery SHALL follow the existing design system at mobile and desktop widths, support keyboard use, 200 percent text zoom and reduced motion, and supply EN/VI interface, state and recovery copy. Published editorial content SHALL remain as authored. Demo records SHALL retain a visible demo label. Lot-detail bidding SHALL require a current commerce session and confirmed eligibility; anonymous discovery/history SHALL require no account or game session. Checkout/orders SHALL remain unavailable.

#### Scenario: Mobile and language switch
- **WHEN** a visitor browses at 320 or 375 CSS pixels, changes EN/VI, or enlarges text to 200 percent
- **THEN** navigation, query controls, gallery, prices and fulfillment remain readable and usable without page-level horizontal overflow
- **AND** language changes preserve loaded records and selections, translate interface labels, and update the document language

#### Scenario: Real record versus demo and future bidding
- **WHEN** a visitor views a stored demo or non-demo lot and attempts the bid control
- **THEN** only demos are labeled as demonstration content, and the bid panel reports the server-confirmed execution state, offering login or deliberate submission only when eligible
- **AND** browsing alone creates no account, watchlist, bid, order or payment mutation, restores no game token and opens no game WebSocket; only deliberate eligible bidding can create a commerce bid

## ADDED Requirements

### Requirement: Separate execution data from discovery selections
Discovery filters and opening-price sorts SHALL retain their existing URL semantics and editorial amounts. Lot detail SHALL load auction execution independently from catalog content. Auction-service failure SHALL leave published content inspectable while clearly disabling bidding and avoiding invented price/history/results.

#### Scenario: Schedule filters with live bidding
- **WHEN** a visitor uses `phase=window` or `sort=price-asc` after bids have raised a lot's current price
- **THEN** the filter still describes the published schedule and sorting still compares opening prices
- **AND** the separate lot-detail execution panel labels current price, bid minimum and committed result distinctly

#### Scenario: Auction service failure
- **WHEN** catalog content loads but the execution snapshot fails, becomes stale or is malformed
- **THEN** gallery, condition, fulfillment and parent-event navigation remain usable with an explicit auction recovery state
- **AND** the bid form stays disabled until fresh execution and identity permit submission

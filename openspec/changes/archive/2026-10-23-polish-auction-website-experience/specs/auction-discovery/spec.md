# Spec Delta

## MODIFIED Requirements

### Requirement: Published discovery homepage
The homepage SHALL display the brand introduction, up to three featured published events, up to six lots nearest their future planned close, and links for all catalog categories. Featured events SHALL prioritize the current planned window, then upcoming, then elapsed, with opening-time and slug tie breaks. All record links SHALL use stored slugs. Empty sections SHALL explain their state without sample replacements. An image-led focal composition SHALL use the first featured published event's media/content with direct exploration links; an empty catalog SHALL use a restrained brand composition without a fabricated event.

#### Scenario: Discover from home
- **WHEN** published events span current, upcoming, and elapsed planned windows
- **THEN** the featured events follow that phase order, with opening time ascending within current/upcoming and descending within elapsed, then slug ascending
- **AND** nearest-close lots are drawn only from the current planned window, ordered by close time ascending then slug, and lead to their stored details

#### Scenario: No current lots or published records
- **WHEN** no lot is within its planned window or the entire public catalog is empty
- **THEN** the applicable homepage section explains that no matching published content is available and offers the auction or lot index
- **AND** category links lead to `/lots` with the corresponding category filter and no fixture records are substituted

#### Scenario: Focal collection presentation
- **WHEN** featured published content is available
- **THEN** the leading event image and story anchor the homepage with clear event and collection links
- **AND** media failure preserves the focal region and navigation rather than inserting an unrelated object or false live claim

### Requirement: Inspectable lot content and gallery
Lot detail SHALL show the matching published title, category, description, condition, optional brand/era, ordered specifications, supplied story and parent event link. Its gallery SHALL preserve published image order and alternative text, offer keyboard-operable image selection and enlarged inspection, and use local failure fallbacks. Missing historical fields SHALL be omitted or identified as unknown, without authenticity claims. Enlarged inspection SHALL support close/previous/next controls, Escape, contained focus and restoration to its opener.

#### Scenario: Inspect all images and details
- **WHEN** a visitor selects gallery images on a published lot with multiple images and specifications
- **THEN** each selected image and its stored alternative text can be inspected in published order using keyboard and pointer controls
- **AND** condition, specifications and supplied story remain accessible without assuming the story is independently verified

#### Scenario: Media becomes unavailable
- **WHEN** an image file is missing after publication
- **THEN** its image region presents the local unavailable fallback and readable explanation
- **AND** the rest of the gallery, content and parent-event navigation remain usable

#### Scenario: Enlarge and return
- **WHEN** a visitor opens an image for enlarged inspection and moves to another image
- **THEN** the gallery and enlarged view share the selected image and expose localized control names and image position
- **AND** closing restores focus without changing lot identity, losing selection, submitting an action or recording another lot view

### Requirement: Accessible bilingual discovery without transactions
Discovery SHALL follow the shared design system at mobile and desktop widths, support keyboard use, actual 200-percent browser zoom and reduced motion, and supply EN/VI interface, state and recovery copy. Published editorial content SHALL remain as authored except for the defined bounded legacy seed presentation. Consumer cards/details SHALL omit demo badges, simulation notices and rule-engine/future-ML announcements. Lot-detail bidding SHALL require a current commerce session and confirmed eligibility; anonymous discovery/history SHALL require no account or game session. Winner checkout, order history and fulfillment detail SHALL link through owner-authenticated reads; public discovery SHALL expose no private purchase or shipping data. Optional first-party view recording SHALL use the separate interaction endpoint without blocking browsing or changing commerce state.

#### Scenario: Mobile and language switch
- **WHEN** a visitor browses at 320 or 375 CSS pixels, changes language, or uses actual 200-percent browser zoom
- **THEN** navigation, query controls, gallery, prices and fulfillment remain readable and usable without page-level horizontal overflow
- **AND** language changes preserve loaded records and selections, translate interface labels, and update the document language

#### Scenario: Real record versus demo and future bidding
- **WHEN** a visitor views a stored demo or non-demo lot and attempts the bid control
- **THEN** both use the same product presentation without demonstration badges, and the bid panel reports the server-confirmed execution state, offering login or deliberate submission only when eligible
- **AND** browsing alone creates no account, watchlist, bid, order or payment mutation, restores no game token and opens no game WebSocket; only deliberate eligible bidding can create a commerce bid and only server closure can create its winner order

#### Scenario: Winner continues from the public lot
- **WHEN** an authenticated winner views a sold lot with confirmed owned participation
- **THEN** links lead to that owner's checkout/confirmation and order history/detail without placing purchase or fulfillment details into the public lot response
- **AND** anonymous/losing visitors see only the public result and public discovery remains usable if checkout is unavailable

## ADDED Requirements

### Requirement: Focused lot auction hierarchy
Lot detail SHALL emphasize the gallery, title and exact current price or explicit no-bids state, followed by closing time, eligibility and the primary bid action. Opening price, fixed increment, bid minimum and history SHALL remain distinctly labeled. Condition and all fulfillment terms SHALL precede submission in reading order. Sticky treatments SHALL not obscure content, errors or focus.

#### Scenario: Inspect terms before bidding
- **WHEN** a visitor reads the lot by keyboard or on a narrow screen
- **THEN** condition, available fulfillment methods, instructions and fees are reachable before the enabled submit control
- **AND** a fixed or sticky bid treatment does not create another submission path that bypasses those disclosures or current eligibility

### Requirement: Compact responsive discovery controls
Lot and event indexes SHALL group search, filters, sorting, result counts and reset actions with clear labels. On narrow layouts secondary filters SHALL be expandable while preserving supported native GET query semantics and current selections. Opening or closing filters SHALL not itself navigate or alter results.

#### Scenario: Mobile filter submission
- **WHEN** a visitor opens filters, chooses supported values and deliberately applies them
- **THEN** the existing URL/query semantics determine results and survive refresh, new tabs and Back/Forward
- **AND** no-results preserves selections and offers reset, distinct from loading, empty catalog and service failure

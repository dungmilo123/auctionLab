# auction-discovery Specification

## Purpose

Let anonymous visitors discover published vintage auction events and lots, inspect their condition and fulfillment terms, and understand planned schedules before bidding becomes available.

## Requirements

### Requirement: Published discovery homepage
The homepage SHALL display the brand introduction, up to three featured published events, up to six lots nearest their future planned close, and links for all catalog categories. Featured events SHALL prioritize the current planned window, then upcoming, then elapsed, with opening-time and slug tie breaks. All record links SHALL use stored slugs. Empty sections SHALL explain their state without sample replacements.

#### Scenario: Discover from home
- **WHEN** published events span current, upcoming, and elapsed planned windows
- **THEN** the featured events follow that phase order, with opening time ascending within current/upcoming and descending within elapsed, then slug ascending
- **AND** nearest-close lots are drawn only from the current planned window, ordered by close time ascending then slug, and lead to their stored details

#### Scenario: No current lots or published records
- **WHEN** no lot is within its planned window or the entire public catalog is empty
- **THEN** the applicable homepage section explains that no matching published content is available and offers the auction or lot index
- **AND** category links lead to `/lots` with the corresponding category filter and no fixture records are substituted

### Requirement: Event browsing and detail
The auction index SHALL offer all, upcoming, within-planned-window, and planned-window-elapsed views of published events. Event detail SHALL show the matching event's title, theme, cover, description, planned opening/end, and all its lots in published position order, with lot detail links. Phase groups SHALL use the same ordering as the homepage.

#### Scenario: Overlapping events and staggered lots
- **WHEN** a visitor opens a published event while another event overlaps its planned window
- **THEN** its own description and complete ordered membership appear with each lot's opening and staggered closing time
- **AND** an event remains within its planned window until its last lot's planned close, even when earlier lot windows have elapsed

#### Scenario: Event phase selection
- **WHEN** a visitor selects a phase on `/auctions` and reloads the resulting URL
- **THEN** the selected phase and matching published events are restored
- **AND** an empty selection displays a no-results message and an all-events recovery action

### Requirement: Searchable lot index
The lot index SHALL combine case-insensitive substring search across title, description, brand and era with category and planned-phase filters using AND semantics. It SHALL offer closing-soon, opening-price ascending, opening-price descending and title sorting. Search SHALL trim surrounding whitespace; absent optional fields SHALL behave as empty text. Results SHALL show a count and a clear-filters action.

#### Scenario: Combined filters
- **WHEN** a visitor searches for a brand substring, selects film cameras and the upcoming planned phase
- **THEN** only published lots satisfying all three conditions appear
- **AND** stored text remains unchanged and safely rendered

#### Scenario: Deterministic sorting
- **WHEN** a visitor sorts matching lots
- **THEN** closing-soon orders future planned closes ascending before elapsed closes descending, price orders integer opening cents in the requested direction, and title orders case-insensitive title ascending
- **AND** every tie ends with slug ascending, missing sort values appear last, and zero cents remains a valid value

#### Scenario: Search has no matches
- **WHEN** the catalog is nonempty but the selected query matches no lot
- **THEN** the page preserves the query and shows a no-results explanation with a clear-filters action
- **AND** it distinguishes no matches from an empty published catalog and from a failed request

### Requirement: Shareable discovery queries
Discovery selections SHALL be represented by `q`, `category`, `phase`, and `sort` on `/lots`, and `phase` on `/auctions`. Accepted phases SHALL be `all`, `upcoming`, `window`, and `elapsed`; sort values SHALL be `closing-soon`, `price-asc`, `price-desc`, and `title`. Missing or invalid selections SHALL default to all phases/categories and closing-soon sorting. Query handling SHALL preserve native browser navigation.

#### Scenario: Bookmark and browser history
- **WHEN** a visitor submits discovery controls, opens the URL in a new tab, refreshes, or returns with Back/Forward
- **THEN** the controls and results reflect the URL after the public records load
- **AND** category values resolve by public category slug, unknown parameters are ignored, and duplicate recognized parameters use their first value

#### Scenario: Invalid query input
- **WHEN** a URL contains an unknown category, unsupported sort/phase, or markup in search text
- **THEN** unsupported selections use their documented defaults and search is treated only as text
- **AND** the page does not crash, execute markup, or request private APIs

### Requirement: Inspectable lot content and gallery
Lot detail SHALL show the matching published title, category, description, condition, optional brand/era, ordered specifications, supplied story and parent event link. Its gallery SHALL preserve published image order and alternative text, offer keyboard-operable image selection, and use local failure fallbacks. Missing historical fields SHALL be omitted or identified as unknown, without authenticity claims.

#### Scenario: Inspect all images and details
- **WHEN** a visitor selects gallery images on a published lot with multiple images and specifications
- **THEN** each selected image and its stored alternative text can be inspected in published order using keyboard and pointer controls
- **AND** condition, specifications and supplied story remain accessible without assuming the story is independently verified

#### Scenario: Media becomes unavailable
- **WHEN** an image file is missing after publication
- **THEN** its image region presents the local unavailable fallback and readable explanation
- **AND** the rest of the gallery, content and parent-event navigation remain usable

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

### Requirement: Recoverable data loading
Public pages SHALL distinguish loading, empty catalog, no matches, missing record, backend unavailable, and other request failures. Failures SHALL provide retry or navigation recovery without substituting fixtures or exposing raw internal errors. A failed request SHALL NOT be presented as an empty successful catalog, and an obsolete request SHALL NOT overwrite a later selection or retry.

#### Scenario: Missing or unpublished detail
- **WHEN** a visitor requests a missing slug or a draft slug
- **THEN** the page shows the same record-not-found message and public index recovery links
- **AND** no generic lot/event preview, private existence information or editable content appears

#### Scenario: Backend unavailable and retry
- **WHEN** a public read fails because of transport failure, malformed success data, or service unavailability
- **THEN** a readable unavailable/error state replaces the loading state and offers retry
- **AND** retry reloads public data, preserves discovery selections, and only a valid successful response produces results

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

### Requirement: Independent recommendation discovery
Home and lot detail SHALL load rule-based suggestions independently from existing catalog and auction execution. Existing homepage featured events, nearest-close sections, query semantics and opening-price sorts SHALL remain intact. Suggestions SHALL use only safe published cards and native stored-slug links.

#### Scenario: Suggestions and observations do not transact
- **WHEN** a visitor loads a published lot or follows a recommendation
- **THEN** related-lot loading and optional view recording create no account, watch entry, bid, result, order or payment
- **AND** a failed recommendation or observation request leaves the loaded gallery, fulfillment terms and eligible deliberate bid controls usable

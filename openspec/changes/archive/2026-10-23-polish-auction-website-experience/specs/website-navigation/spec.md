# Spec Delta

## MODIFIED Requirements

### Requirement: Navigable layout shells
The storefront SHALL have brand navigation, links to auctions, lots and account, accessible search and watchlist entry points, a main content landmark and a footer. Account and admin shells SHALL add icon-supported contextual navigation and a route back to the storefront. Every navigation link SHALL resolve to an implemented destination, identify the current destination accessibly and remain usable on mobile. Narrow layouts SHALL provide an operable collapsible navigation panel without concealing essential destinations.

#### Scenario: Navigating between website areas
- **WHEN** a visitor follows storefront links, account section links, or authorized admin section links
- **THEN** each destination renders the appropriate shell and page heading
- **AND** the active destination is identifiable, a skip link reaches main content, and the footer remains available

#### Scenario: Browser navigation semantics
- **WHEN** a visitor opens a link in a new tab or uses browser Back and Forward after following links
- **THEN** the browser displays the expected destination and URL without losing navigation availability

#### Scenario: Narrow navigation and search
- **WHEN** a visitor opens the mobile navigation or header search affordance
- **THEN** existing destinations remain keyboard reachable and search submits to `/lots` using its existing `q` query
- **AND** Escape closes an open panel, focus returns to its trigger and no menu action fabricates a badge count or changes authentication

### Requirement: Honest preview boundaries
Website copy SHALL describe the user's task and actual committed state without repeated capability inventories, demo/preview badges, simulation/no-charge disclaimers or future-ML announcements in storefront and buyer flows. This presentation SHALL NOT alter internal demo provenance, simulated payment/fulfillment behavior, server-confirmed eligibility, owner privacy, capability authorization or game isolation. Dedicated operator analytics provenance controls and technical documentation SHALL retain their factual data definitions.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses public discovery, account pages, or order history
- **THEN** working account/watchlist/bid/checkout/fulfillment actions report only their confirmed result and unavailable actions explain the relevant recovery
- **AND** winning a sold lot produces one awaiting-payment order and never implies payment, regardless of presentation labels

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

#### Scenario: Working catalog administration
- **WHEN** an authorized operator saves a lot draft or publishes an auction event
- **THEN** catalog management reports only the committed operation with concise task-specific feedback
- **AND** the next successful public discovery read reflects committed publication without creating a game connection, account session, watchlist entry or bid; separate auction execution adopts the published schedule and later creates an order only for a sold result

#### Scenario: Public notices reflect stored data
- **WHEN** a visitor browses a demo or non-demo published event or lot and continues to checkout or a receipt
- **THEN** the shared shell and transaction flow omit demonstration badges, simulation/no-charge warnings and implementation-progress or future-ML notices
- **AND** stored provenance and method values remain unchanged, images describe the content available, and the UI makes no gateway settlement, verified-authenticity or carrier-confirmation claim unsupported by the system

#### Scenario: Account service unavailable during discovery
- **WHEN** account state cannot be loaded but the published catalog read succeeds
- **THEN** discovery remains usable and watch controls identify their account-service recovery state
- **AND** the site does not present service failure as a confirmed signed-out identity or activate bidding

#### Scenario: Operator data provenance
- **WHEN** an authorized operator filters analytics or inspects metric definitions
- **THEN** demo/non-demo cohorts, simulated-value definitions and coverage limitations remain available in that operational context
- **AND** those definitions do not become repeated global announcements or buyer badges

## ADDED Requirements

### Requirement: Native-name language dropdown
The shared website language control SHALL show the current native language name with a globe and expansion indicator, and offer English and Tiếng Việt with selected-state feedback. It SHALL support keyboard selection, Escape and focus return, preserve the existing preference and page/query/form state, update document language and submit no commerce action. Country flags SHALL NOT be the sole identifier.

#### Scenario: Switch while editing or filtering
- **WHEN** a visitor changes language on a filtered index, an active owned checkout or an authorized shipping form
- **THEN** the interface and accessible names change, the selection persists across navigation and the same query, selected image and valid active form values remain
- **AND** no document navigation, bid, save, review, payment, fulfillment or view-recording event is caused solely by the language change

#### Scenario: Operate without pointer
- **WHEN** the visitor opens language selection using the keyboard
- **THEN** available languages and the selected language are announced and can be chosen without a pointer
- **AND** cancelling closes it and restores trigger focus without changing language

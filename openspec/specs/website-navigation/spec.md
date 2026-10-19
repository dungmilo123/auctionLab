# website-navigation Specification

## Purpose

Give AuctionLab a predictable website navigation structure across public storefront, account, and administration areas, with working deep links and explicit boundaries during the transition from the classroom game.

## Requirements

### Requirement: Explicit website destinations
The website SHALL recognize only the routes listed below. Public detail routes SHALL match one nonempty slug segment and resolve the corresponding published record. Matching SHALL be case-sensitive, ignore query and fragment for route identity, and accept one trailing slash. Unrecognized paths SHALL render the not-found view rather than a game or an unrelated page. Catalog administration SHALL select its editor using query parameters within existing routes.

#### Scenario: Storefront destinations
- **WHEN** a visitor opens `/`, `/auctions`, `/auctions/:slug`, `/lots`, or `/lots/:slug`
- **THEN** the corresponding published discovery home, event index, event detail, lot index, or lot detail appears within the storefront layout after loading
- **AND** a detail slug selects its stored public record or a record-not-found state rather than a generic preview

#### Scenario: Account destinations
- **WHEN** a visitor opens `/account`, `/account/watchlist`, `/account/bids`, or `/account/orders`
- **THEN** the corresponding account preview appears with account navigation
- **AND** no private information or simulated authenticated identity is displayed

#### Scenario: Administration destinations
- **WHEN** an authorized operator opens `/admin`, `/admin/auctions`, `/admin/lots`, or `/admin/orders` through the Go server
- **THEN** the admin overview, working auction-event management, working lot management, or order preview appears respectively with admin navigation
- **AND** catalog editors require authorized catalog API access independently of the document gate

#### Scenario: Exact matching and normalization
- **WHEN** a visitor opens `/lots/sample-camera/?view=preview#details`
- **THEN** the lot detail route resolves the published `sample-camera` record or its record-not-found state
- **AND** `/administrator`, `/account/unknown`, `/lots/sample-camera/extra`, and `/Lots` render the not-found view

### Requirement: Navigable layout shells
The storefront SHALL have brand navigation, links to auctions, lots, and account, a main content landmark, and a footer. Account and admin shells SHALL add contextual navigation and a route back to the storefront. Every displayed navigation link SHALL resolve to an implemented foundation destination, identify the current destination accessibly, and remain usable on mobile.

#### Scenario: Navigating between website areas
- **WHEN** a visitor follows storefront links, account section links, or authorized admin section links
- **THEN** each destination renders the appropriate shell and page heading
- **AND** the active destination is identifiable, a skip link reaches main content, and the footer remains available

#### Scenario: Browser navigation semantics
- **WHEN** a visitor opens a link in a new tab or uses browser Back and Forward after following links
- **THEN** the browser displays the expected destination and URL without losing navigation availability

### Requirement: Direct URL loading
Recognized website routes SHALL render after direct entry, bookmark navigation, or refresh in the development frontend and embedded Go build. Production admin routes SHALL first pass the existing server gate. Public discovery SHALL load its published selection and URL query or an explicit recovery state. Catalog management SHALL load its authorized saved selection or an explicit access/error state. Website documents SHALL retain a no-cache policy in the embedded build.

#### Scenario: Public nested page refresh
- **WHEN** a visitor directly loads or refreshes `/auctions/analog-memories`, `/lots/sample-camera`, or `/account/watchlist`
- **THEN** the matching published record or record-not-found view, or account preview respectively, loads without requiring a prior visit to `/`
- **AND** the embedded build returns the SPA document with `Cache-Control: no-cache`

#### Scenario: Authorized admin nested page refresh
- **WHEN** an operator with a valid admin capability cookie refreshes `/admin/lots` on the Go server
- **THEN** stored lot management loads at that URL
- **AND** an `edit` query selection loads the saved record or a recoverable record-not-found state

#### Scenario: Development catalog access
- **WHEN** an operator directly opens a catalog management page on the development frontend
- **THEN** it loads stored records only through authorized APIs or shows an explicit access/backend error state
- **AND** it never treats the publicly served frontend document as authorization

#### Scenario: Discovery query deep link
- **WHEN** a visitor directly opens `/lots?q=camera&category=film-cameras&phase=upcoming&sort=price-asc` or returns to it with browser history
- **THEN** its public discovery controls and results restore the supported query selections after loading
- **AND** normal document links still support refresh, new tabs, Back and Forward

### Requirement: Preserve the admin capability boundary
The Go server SHALL retain the existing key and cookie gate on `/admin` and its subtree until account authorization replaces it in a later change. Its entry page SHALL use the vintage palette, readable typography, and accessible controls without external network dependencies. Website preview links SHALL not expose keys or authorize admin operations.

#### Scenario: Missing or invalid capability
- **WHEN** a browser without a valid capability requests an admin URL on the Go server
- **THEN** it receives the gate document instead of the admin SPA document
- **AND** an invalid nonempty key produces the existing unauthorized response without echoing the key

#### Scenario: Successful key entry
- **WHEN** a browser supplies a correct admin key on the Go server
- **THEN** the existing capability cookie is set and the browser redirects to `/admin` with the key removed from the URL
- **AND** the operator can subsequently navigate to the admin preview destinations

### Requirement: Honest preview boundaries
Public discovery SHALL identify stored published content, demos, planned schedules and unavailable transactions accurately. Account and order-preview pages SHALL identify illustrative content and unavailable features. Website pages SHALL avoid game sessions and game WebSocket connections. Only authorized catalog administration SHALL mutate catalog data; account, bid, watchlist, payment and order mutations SHALL remain unavailable, with future controls omitted or disabled and explained.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses public discovery, an account preview, or an order preview, or activates a preview-only control
- **THEN** the website does not join a game, submit a business mutation, or show a fabricated successful transaction
- **AND** published public records and labeled demos are distinguishable from illustrative account/order content and unavailable live commerce data

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

#### Scenario: Working catalog administration
- **WHEN** an authorized operator saves a lot draft or publishes an auction event
- **THEN** catalog management reports only the committed catalog operation and indicates that bidding and purchases are unavailable
- **AND** the next successful public discovery read reflects committed publication while account/order previews remain unchanged and no game connection or commerce transaction is created

#### Scenario: Public notices reflect stored data
- **WHEN** a visitor browses a non-demo published event or lot
- **THEN** shared notices and footer copy do not describe the record as an imagined object, generic preview, or necessarily an illustration
- **AND** the website still explains that bidding and purchases are unavailable and labels demo records when applicable

### Requirement: Retire the old game route surface
The website SHALL replace the player experience at `/` and the game control room at `/admin`. `/screen` and all its descendants SHALL render the not-found view. Foundation navigation SHALL not expose a legacy player, projector, or control-room route alias. This change SHALL not delete existing game data.

#### Scenario: Opening an old projector bookmark
- **WHEN** a visitor opens `/screen` or `/screen/anything/deep`
- **THEN** the not-found view appears with a link to the storefront
- **AND** no projector connection, QR game join, podium, or game controls are mounted

### Requirement: Recoverable not-found view
Unrecognized document URLs SHALL display an explicit 404 page with a descriptive title and a usable link to `/`. It SHALL share the website theme and navigation. Because the frontend resolves document routes, the SPA document response MAY remain HTTP 200; the page SHALL not claim server-side resource discovery.

#### Scenario: Recovering from an unknown URL
- **WHEN** a visitor opens `/unknown-page` or, after admin authorization, `/admin/unknown-page`
- **THEN** the website displays the 404 view instead of a blank screen or the player game
- **AND** following its home link returns to the storefront

### Requirement: Asset failures remain asset failures
Missing requests in reserved static asset namespaces SHALL return HTTP 404 rather than the SPA document in both development and the embedded build. Existing application assets SHALL be served locally; only content-hashed build assets SHALL receive immutable caching. API and WebSocket endpoints SHALL retain their server behavior rather than being routed as website pages.

#### Scenario: Missing bundled font or illustration
- **WHEN** a browser requests a nonexistent font or image under `/assets/` or `/site-assets/`
- **THEN** the server returns HTTP 404 without the SPA document body
- **AND** a missing asset response is not cached as immutable content

#### Scenario: Existing hashed build asset
- **WHEN** a browser requests an existing content-hashed file under `/assets/` from the embedded build
- **THEN** it receives that asset with the existing immutable cache policy
- **AND** a real non-hashed public file does not receive that immutable policy

#### Scenario: Unknown API request
- **WHEN** a client requests an unrecognized `/api/` endpoint
- **THEN** it receives the existing API error response rather than a website preview

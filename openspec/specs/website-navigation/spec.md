# website-navigation Specification

## Purpose

Give AuctionLab a predictable website navigation structure across public storefront, account, and administration areas, with working deep links and explicit boundaries during the transition from the classroom game.

## Requirements

### Requirement: Explicit website destinations
The website SHALL recognize only the foundation routes listed below. Detail placeholders SHALL match one nonempty slug segment. Matching SHALL be case-sensitive, ignore query and fragment for route identity, and accept one trailing slash. Unrecognized paths SHALL render the not-found view rather than a game or an unrelated page.

#### Scenario: Storefront destinations
- **WHEN** a visitor opens `/`, `/auctions`, `/auctions/:slug`, `/lots`, or `/lots/:slug`
- **THEN** the corresponding home, auction index, auction detail, lot index, or lot detail preview appears within the storefront layout
- **AND** a slug selects a generic detail preview without asserting that a stored auction or lot exists

#### Scenario: Account destinations
- **WHEN** a visitor opens `/account`, `/account/watchlist`, `/account/bids`, or `/account/orders`
- **THEN** the corresponding account preview appears with account navigation
- **AND** no private information or simulated authenticated identity is displayed

#### Scenario: Administration destinations
- **WHEN** an authorized operator opens `/admin`, `/admin/auctions`, `/admin/lots`, or `/admin/orders` through the Go server
- **THEN** the corresponding administration preview appears with admin navigation

#### Scenario: Exact matching and normalization
- **WHEN** a visitor opens `/lots/sample-camera/?view=preview#details`
- **THEN** the lot detail preview appears
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
Recognized website routes SHALL render after direct entry, bookmark navigation, or refresh in the development frontend and embedded Go build. Production admin routes SHALL first pass the existing server gate. Website documents SHALL retain a no-cache policy in the embedded build.

#### Scenario: Public nested page refresh
- **WHEN** a visitor directly loads or refreshes `/auctions/analog-memories`, `/lots/sample-camera`, or `/account/watchlist`
- **THEN** the expected preview page loads without requiring a prior visit to `/`
- **AND** the embedded build returns the SPA document with `Cache-Control: no-cache`

#### Scenario: Authorized admin nested page refresh
- **WHEN** an operator with a valid admin capability cookie refreshes `/admin/lots` on the Go server
- **THEN** the administration lot preview loads at that URL

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
Foundation pages SHALL clearly state that their content is a website preview and business features are unavailable. They SHALL use isolated illustrative content, avoid game sessions and game WebSocket connections, and perform no catalog, account, bid, watchlist, payment, or order mutations. Future transaction controls SHALL be omitted or disabled with an explanation.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses any foundation page or activates a preview-only control
- **THEN** the website does not join a game, submit a business request, or show a fabricated successful transaction
- **AND** example content is distinguishable from stored or live commerce data

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

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

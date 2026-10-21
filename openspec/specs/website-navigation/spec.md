# website-navigation Specification

## Purpose

Give AuctionLab a predictable website navigation structure across public storefront, account, and administration areas, with working deep links and explicit boundaries during the transition from the classroom game.

## Requirements

### Requirement: Explicit website destinations
The website SHALL recognize only the routes listed below. Public detail routes SHALL match one nonempty slug segment and resolve the corresponding published record. Matching SHALL be case-sensitive, ignore query and fragment for route identity, and accept one trailing slash. Unrecognized paths SHALL render the not-found view rather than a game or an unrelated page. Catalog administration SHALL select its editor using query parameters within existing routes. Checkout and owner detail SHALL match `/account/checkout/:orderId` and `/account/orders/:orderId` using exactly one 32-character lowercase hexadecimal ID. `/account/wins` SHALL list owned unpaid wins; `/account/orders` SHALL list owned purchase history. Admin order selection SHALL use `/admin/orders?view=<orderId>`.

#### Scenario: Storefront destinations
- **WHEN** a visitor opens `/`, `/auctions`, `/auctions/:slug`, `/lots`, or `/lots/:slug`
- **THEN** the corresponding published discovery home, event index, event detail, lot index, or lot detail appears within the storefront layout after loading
- **AND** a detail slug selects its stored public record or a record-not-found state rather than a generic preview

#### Scenario: Account destinations
- **WHEN** a visitor opens `/account`, `/account/watchlist`, `/account/bids`, `/account/wins`, `/account/checkout/:orderId`, `/account/orders`, or `/account/orders/:orderId`
- **THEN** `/account` resolves a real session and safe profile or a signed-out introduction, and `/account/watchlist` and `/account/bids` resolve their session-owned saved list or accepted bid participation, or a login-required state
- **AND** wins and checkout load only owned orders or explicit login/not-found/error states; orders/detail resolve owner-only history and fulfillment progress, and bids never show another account's data

#### Scenario: Authentication destinations
- **WHEN** a visitor opens `/account/login` or `/account/register`
- **THEN** the matching labeled email/password form appears within the account shell with native links between the forms
- **AND** the form offers a safe return destination and explicit validation, pending, or service-failure states

#### Scenario: Administration destinations
- **WHEN** an authorized operator opens `/admin`, `/admin/auctions`, `/admin/lots`, or `/admin/orders` through the Go server
- **THEN** the admin overview, working auction-event management, working lot management, or working order-management workspace appears respectively with admin navigation
- **AND** catalog editors and order operations require independently authorized APIs as well as the document gate

#### Scenario: Exact matching and normalization
- **WHEN** a visitor opens `/lots/sample-camera/?view=preview#details`
- **THEN** the lot detail route resolves the published `sample-camera` record or its record-not-found state
- **AND** `/administrator`, `/account/unknown`, `/account/login/extra`, `/lots/sample-camera/extra`, and `/Lots` render the not-found view

#### Scenario: Direct bid participation destination
- **WHEN** a user opens or refreshes `/account/bids` in Vite or the embedded build, or returns through browser history
- **THEN** the page validates the current session and loads only its accepted participation or an explicit login/error/empty state
- **AND** obsolete private state is cleared after logout and native links to the published lots remain usable

#### Scenario: Checkout deep link and paid refresh
- **WHEN** a winner directly opens or refreshes `/account/checkout/<orderId>` in Vite or the embedded build
- **THEN** it validates the session and displays that owned order as awaiting payment or paid confirmation
- **AND** unknown/malformed route shapes render not-found, foreign orders reveal no data, and native links support Back/Forward and new tabs

#### Scenario: Exact history detail destination
- **WHEN** a buyer opens `/account/orders/<32-lowercase-hex-id>/` with query/fragment or returns through browser history
- **THEN** it selects only that owned order after current-session validation or shows login/not-found/error recovery
- **AND** malformed IDs, uppercase route names and additional path segments render not-found without opening another order

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
Recognized website routes SHALL render after direct entry, bookmark navigation, or refresh in the development frontend and embedded Go build. Production admin routes SHALL first pass the existing server gate. Public discovery SHALL load its published selection and URL query or an explicit recovery state. Catalog and order management SHALL load their authorized saved selection or an explicit access/error state; owner history/detail SHALL validate the commerce session and load only owned purchases. Website documents SHALL retain a no-cache policy in the embedded build.

#### Scenario: Public nested page refresh
- **WHEN** a visitor directly loads or refreshes `/auctions/analog-memories`, `/lots/sample-camera`, or `/account/watchlist`
- **THEN** the matching published record or record-not-found view, or session-owned watchlist/login-required state respectively, loads without requiring a prior visit to `/`
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

#### Scenario: Account refresh and history after logout
- **WHEN** a browser directly loads an authentication destination or navigates Back to an account page after logout
- **THEN** the page checks the current server session before showing private data
- **AND** restored pages clear stale private state and show login-required recovery when the session is invalid

#### Scenario: Direct owner and admin order links
- **WHEN** a buyer opens `/account/orders` or its detail URL, or an operator opens `/admin/orders?view=<orderId>`, in Vite or the embedded build
- **THEN** each destination loads the appropriate authorized stored collection/detail and supports refresh, native links, new tabs and Back/Forward
- **AND** the Go admin document gate and independent API guards remain enforced

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
Public discovery SHALL accurately identify published content, demos, planned schedules, and unavailable transactions. Accounts, watchlists and bid participation SHALL show only server-confirmed identity and owned data. Lot-detail bidding SHALL require current commerce eligibility; owned wins and single-order checkout SHALL support explicitly simulated payment, with owner-only order history and capability-authorized simulated fulfillment available through their private routes. Website pages SHALL avoid game sessions and game WebSockets. Catalog mutation SHALL remain capability-authorized.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses public discovery, account pages, or order history, or activates a preview-only control
- **THEN** working account/watchlist/bid/checkout/fulfillment actions report only their confirmed result and preview-only controls create no transaction or game connection
- **AND** published records and labeled demos remain distinguishable, and winning a sold lot produces one awaiting-payment order and never implies payment

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

#### Scenario: Working catalog administration
- **WHEN** an authorized operator saves a lot draft or publishes an auction event
- **THEN** catalog management reports only the committed catalog operation and explains scheduled commerce bidding, private winner checkout/history with simulated payment and dedicated simulated fulfillment operations
- **AND** the next successful public discovery read reflects committed publication without creating a game connection, account session, watchlist entry or bid; separate auction execution adopts the published schedule and later creates an order only for a sold result

#### Scenario: Public notices reflect stored data
- **WHEN** a visitor browses a non-demo published event or lot
- **THEN** shared notices and footer copy do not describe the record as an imagined object, generic preview, or necessarily an illustration
- **AND** notices distinguish available accounts/watchlists/scheduled bidding/private checkout/history/simulated fulfillment from future analytics and recommendations, identify every payment as simulated, and label demo records when applicable

#### Scenario: Account service unavailable during discovery
- **WHEN** account state cannot be loaded but the published catalog read succeeds
- **THEN** discovery remains usable and watch controls identify their account-service recovery state
- **AND** the site does not present service failure as a confirmed signed-out identity or activate bidding

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
### Requirement: Safe authentication return navigation
Authentication links SHALL use a `return_to` destination limited to recognized storefront or account pages on this site. Invalid, external, admin, API, WebSocket, asset, authentication-loop, or malformed destinations SHALL fall back to `/account`. Successful login/registration SHALL navigate only after confirmed success and SHALL NOT replay a save, bid or payment confirmation automatically.

#### Scenario: Return to the saved-lot context
- **WHEN** a visitor logs in from `/account/login?return_to=%2Flots%2Fsample-camera`
- **THEN** confirmed login navigates to the matching local lot URL
- **AND** the visitor explicitly chooses whether to save it

#### Scenario: Reject unsafe return target
- **WHEN** `return_to` contains a scheme, protocol-relative URL, backslash, control character, malformed encoding, or excluded destination
- **THEN** authentication uses `/account` as its destination
- **AND** the supplied target is never used as an unchecked redirect or HTML value

#### Scenario: Return to checkout after login
- **WHEN** a signed-out visitor logs in with a recognized local checkout or unpaid-wins return destination
- **THEN** the destination reloads and revalidates ownership before showing purchase data
- **AND** unsent contact/address data is not encoded in the URL or restored across identities and confirmation requires a new deliberate action

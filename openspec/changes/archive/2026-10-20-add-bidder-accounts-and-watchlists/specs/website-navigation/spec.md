# Spec Delta

## MODIFIED Requirements

### Requirement: Explicit website destinations
The website SHALL recognize only the routes listed below. Public detail routes SHALL match one nonempty slug segment and resolve the corresponding published record. Matching SHALL be case-sensitive, ignore query and fragment for route identity, and accept one trailing slash. Unrecognized paths SHALL render the not-found view rather than a game or an unrelated page. Catalog administration SHALL select its editor using query parameters within existing routes.

#### Scenario: Storefront destinations
- **WHEN** a visitor opens `/`, `/auctions`, `/auctions/:slug`, `/lots`, or `/lots/:slug`
- **THEN** the corresponding published discovery home, event index, event detail, lot index, or lot detail appears within the storefront layout after loading
- **AND** a detail slug selects its stored public record or a record-not-found state rather than a generic preview

#### Scenario: Account destinations
- **WHEN** a visitor opens `/account`, `/account/watchlist`, `/account/bids`, or `/account/orders`
- **THEN** `/account` resolves a real session and safe profile or a signed-out introduction, and `/account/watchlist` resolves the private list or login-required state
- **AND** bids and orders remain explicitly unavailable previews without invented activity or another account's data

#### Scenario: Authentication destinations
- **WHEN** a visitor opens `/account/login` or `/account/register`
- **THEN** the matching labeled email/password form appears within the account shell with native links between the forms
- **AND** the form offers a safe return destination and explicit validation, pending, or service-failure states

#### Scenario: Administration destinations
- **WHEN** an authorized operator opens `/admin`, `/admin/auctions`, `/admin/lots`, or `/admin/orders` through the Go server
- **THEN** the admin overview, working auction-event management, working lot management, or order preview appears respectively with admin navigation
- **AND** catalog editors require authorized catalog API access independently of the document gate

#### Scenario: Exact matching and normalization
- **WHEN** a visitor opens `/lots/sample-camera/?view=preview#details`
- **THEN** the lot detail route resolves the published `sample-camera` record or its record-not-found state
- **AND** `/administrator`, `/account/unknown`, `/account/login/extra`, `/lots/sample-camera/extra`, and `/Lots` render the not-found view

### Requirement: Direct URL loading
Recognized website routes SHALL render after direct entry, bookmark navigation, or refresh in the development frontend and embedded Go build. Production admin routes SHALL first pass the existing server gate. Public discovery SHALL load its published selection and URL query or an explicit recovery state. Catalog management SHALL load its authorized saved selection or an explicit access/error state. Website documents SHALL retain a no-cache policy in the embedded build.

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

### Requirement: Honest preview boundaries
Public discovery SHALL accurately identify published content, demos, planned schedules, and unavailable transactions. Accounts and watchlists SHALL show only server-confirmed identity and saved data. Bids, payments, and orders SHALL remain unavailable with omitted or disabled explained controls. Website pages SHALL avoid game sessions and game WebSockets. Catalog mutation SHALL remain capability-authorized.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses public discovery, account pages, or an order preview, or activates a preview-only control
- **THEN** working account/watchlist actions report only their confirmed result and preview-only controls create no transaction or game connection
- **AND** published records and labeled demos remain distinguishable from unavailable bid/order activity

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

#### Scenario: Working catalog administration
- **WHEN** an authorized operator saves a lot draft or publishes an auction event
- **THEN** catalog management reports only the committed catalog operation and indicates that bidding and purchases are unavailable
- **AND** the next successful public discovery read reflects committed publication without creating a game connection, account session, watchlist entry, or commerce transaction

#### Scenario: Public notices reflect stored data
- **WHEN** a visitor browses a non-demo published event or lot
- **THEN** shared notices and footer copy do not describe the record as an imagined object, generic preview, or necessarily an illustration
- **AND** notices distinguish available accounts/watchlists from unavailable bidding and purchases and label demo records when applicable

#### Scenario: Account service unavailable during discovery
- **WHEN** account state cannot be loaded but the published catalog read succeeds
- **THEN** discovery remains usable and watch controls identify their account-service recovery state
- **AND** the site does not present service failure as a confirmed signed-out identity or activate bidding

## ADDED Requirements

### Requirement: Safe authentication return navigation
Authentication links SHALL use a `return_to` destination limited to recognized storefront or account pages on this site. Invalid, external, admin, API, WebSocket, asset, authentication-loop, or malformed destinations SHALL fall back to `/account`. Successful login/registration SHALL navigate only after confirmed success and SHALL NOT replay a save or bid automatically.

#### Scenario: Return to the saved-lot context
- **WHEN** a visitor logs in from `/account/login?return_to=%2Flots%2Fsample-camera`
- **THEN** confirmed login navigates to the matching local lot URL
- **AND** the visitor explicitly chooses whether to save it

#### Scenario: Reject unsafe return target
- **WHEN** `return_to` contains a scheme, protocol-relative URL, backslash, control character, malformed encoding, or excluded destination
- **THEN** authentication uses `/account` as its destination
- **AND** the supplied target is never used as an unchecked redirect or HTML value

# Spec Delta

## MODIFIED Requirements

### Requirement: Explicit website destinations
The website SHALL recognize only the routes listed below. Public detail placeholders SHALL match one nonempty slug segment. Matching SHALL be case-sensitive, ignore query and fragment for route identity, and accept one trailing slash. Unrecognized paths SHALL render the not-found view rather than a game or an unrelated page. Catalog administration SHALL select its editor using query parameters within existing routes.

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
- **THEN** the admin overview, working auction-event management, working lot management, or order preview appears respectively with admin navigation
- **AND** catalog editors require authorized catalog API access independently of the document gate

#### Scenario: Exact matching and normalization
- **WHEN** a visitor opens `/lots/sample-camera/?view=preview#details`
- **THEN** the lot detail preview appears
- **AND** `/administrator`, `/account/unknown`, `/lots/sample-camera/extra`, and `/Lots` render the not-found view

### Requirement: Direct URL loading
Recognized website routes SHALL render after direct entry, bookmark navigation, or refresh in the development frontend and embedded Go build. Production admin routes SHALL first pass the existing server gate. Catalog management SHALL load its authorized saved selection or an explicit access/error state. Website documents SHALL retain a no-cache policy in the embedded build.

#### Scenario: Public nested page refresh
- **WHEN** a visitor directly loads or refreshes `/auctions/analog-memories`, `/lots/sample-camera`, or `/account/watchlist`
- **THEN** the expected preview page loads without requiring a prior visit to `/`
- **AND** the embedded build returns the SPA document with `Cache-Control: no-cache`

#### Scenario: Authorized admin nested page refresh
- **WHEN** an operator with a valid admin capability cookie refreshes `/admin/lots` on the Go server
- **THEN** stored lot management loads at that URL
- **AND** an `edit` query selection loads the saved record or a recoverable record-not-found state

#### Scenario: Development catalog access
- **WHEN** an operator directly opens a catalog management page on the development frontend
- **THEN** it loads stored records only through authorized APIs or shows an explicit access/backend error state
- **AND** it never treats the publicly served frontend document as authorization

### Requirement: Honest preview boundaries
Public, account, and order-preview pages SHALL identify illustrative content and unavailable business features. Catalog administration SHALL describe its working content-management actions accurately. Website pages SHALL avoid game sessions and game WebSocket connections. Only authorized catalog administration SHALL mutate catalog data; account, bid, watchlist, payment, and order mutations SHALL remain unavailable, with future controls omitted or disabled and explained.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses a public, account, or order-preview page or activates a preview-only control
- **THEN** the website does not join a game, submit a business request, or show a fabricated successful transaction
- **AND** example content is distinguishable from stored or live commerce data

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

#### Scenario: Working catalog administration
- **WHEN** an authorized operator saves a lot draft or publishes an auction event
- **THEN** catalog management reports only the committed catalog operation and indicates that bidding and purchases are unavailable
- **AND** the public/account/order previews remain unchanged and no game connection or commerce transaction is created

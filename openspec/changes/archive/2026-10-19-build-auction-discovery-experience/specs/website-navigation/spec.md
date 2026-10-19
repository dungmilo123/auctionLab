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

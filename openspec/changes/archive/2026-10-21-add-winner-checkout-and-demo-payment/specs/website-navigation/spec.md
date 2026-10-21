# Spec Delta

## MODIFIED Requirements

### Requirement: Explicit website destinations
The website SHALL recognize only the routes listed below. Public detail routes SHALL match one nonempty slug segment and resolve the corresponding published record. Matching SHALL be case-sensitive, ignore query and fragment for route identity, and accept one trailing slash. Unrecognized paths SHALL render the not-found view rather than a game or an unrelated page. Catalog administration SHALL select its editor using query parameters within existing routes. Checkout SHALL match `/account/checkout/:orderId` with one nonempty order-ID segment; `/account/wins` SHALL list owned unpaid wins.

#### Scenario: Storefront destinations
- **WHEN** a visitor opens `/`, `/auctions`, `/auctions/:slug`, `/lots`, or `/lots/:slug`
- **THEN** the corresponding published discovery home, event index, event detail, lot index, or lot detail appears within the storefront layout after loading
- **AND** a detail slug selects its stored public record or a record-not-found state rather than a generic preview

#### Scenario: Account destinations
- **WHEN** a visitor opens `/account`, `/account/watchlist`, `/account/bids`, `/account/wins`, `/account/checkout/:orderId`, or `/account/orders`
- **THEN** `/account` resolves a real session and safe profile or a signed-out introduction, and `/account/watchlist` and `/account/bids` resolve their session-owned saved list or accepted bid participation, or a login-required state
- **AND** wins and checkout load only owned orders or explicit login/not-found/error states; `/account/orders` remains a Change 7 preview and bids never show another account's data

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

#### Scenario: Direct bid participation destination
- **WHEN** a user opens or refreshes `/account/bids` in Vite or the embedded build, or returns through browser history
- **THEN** the page validates the current session and loads only its accepted participation or an explicit login/error/empty state
- **AND** obsolete private state is cleared after logout and native links to the published lots remain usable

#### Scenario: Checkout deep link and paid refresh
- **WHEN** a winner directly opens or refreshes `/account/checkout/<orderId>` in Vite or the embedded build
- **THEN** it validates the session and displays that owned order as awaiting payment or paid confirmation
- **AND** unknown/malformed route shapes render not-found, foreign orders reveal no data, and native links support Back/Forward and new tabs

### Requirement: Honest preview boundaries
Public discovery SHALL accurately identify published content, demos, planned schedules, and unavailable transactions. Accounts, watchlists and bid participation SHALL show only server-confirmed identity and owned data. Lot-detail bidding SHALL require current commerce eligibility; owned wins and single-order checkout SHALL support explicitly simulated payment, while order history/administration and fulfillment SHALL remain unavailable previews. Website pages SHALL avoid game sessions and game WebSockets. Catalog mutation SHALL remain capability-authorized.

#### Scenario: Exploring the foundation
- **WHEN** a visitor browses public discovery, account pages, or an order preview, or activates a preview-only control
- **THEN** working account/watchlist/bid/checkout actions report only their confirmed result and preview-only controls create no transaction or game connection
- **AND** published records and labeled demos remain distinguishable, and winning a sold lot produces one awaiting-payment order and never implies payment

#### Scenario: Existing game identity in browser storage
- **WHEN** a browser with a saved game token opens the new storefront or account shell
- **THEN** the token is not restored as an account identity and game budget or secret item values do not appear

#### Scenario: Working catalog administration
- **WHEN** an authorized operator saves a lot draft or publishes an auction event
- **THEN** catalog management reports only the committed catalog operation and explains scheduled commerce bidding, private winner checkout with simulated payment and unavailable order history/fulfillment
- **AND** the next successful public discovery read reflects committed publication without creating a game connection, account session, watchlist entry or bid; separate auction execution adopts the published schedule and later creates an order only for a sold result

#### Scenario: Public notices reflect stored data
- **WHEN** a visitor browses a non-demo published event or lot
- **THEN** shared notices and footer copy do not describe the record as an imagined object, generic preview, or necessarily an illustration
- **AND** notices distinguish available accounts/watchlists/scheduled bidding/private winner checkout from unavailable order history and fulfillment, identify every payment as simulated, and label demo records when applicable

#### Scenario: Account service unavailable during discovery
- **WHEN** account state cannot be loaded but the published catalog read succeeds
- **THEN** discovery remains usable and watch controls identify their account-service recovery state
- **AND** the site does not present service failure as a confirmed signed-out identity or activate bidding

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

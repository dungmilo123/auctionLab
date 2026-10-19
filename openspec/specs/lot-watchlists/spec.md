# lot-watchlists Specification

## Purpose

Let signed-in participants privately save published vintage lots and return to those lots across visits without implying a bid, purchase, or auction outcome.

## Requirements

### Requirement: Session-owned watchlists
Watchlist reads and writes SHALL require a valid commerce session and derive the owner solely from that session. Another account's saved membership SHALL NOT be readable or mutable. An admin capability alone SHALL NOT grant access to bidder watchlists.

#### Scenario: Separate participants
- **WHEN** account A saves a lot and account B opens its own watchlist or removes the same lot
- **THEN** B sees only B's entries and A's entry remains unchanged
- **AND** neither client-supplied owner IDs nor a catalog capability allow reading or editing A's list

#### Scenario: Anonymous or expired session
- **WHEN** a watchlist API is called without a valid session
- **THEN** it returns no-store JSON HTTP 401 without memberships or lot data
- **AND** the website explains the login requirement without inventing locally saved entries

### Requirement: Published lot membership
Only a lot in a published event SHALL be addable to a watchlist. Draft and nonexistent targets SHALL return indistinguishable HTTP 404 responses. A saved lot SHALL remain saved when its planned window elapses, without being labeled sold, won, or bid on.

#### Scenario: Save before or after the planned window
- **WHEN** an authenticated user saves an upcoming, within-window, or elapsed published lot
- **THEN** the same stable catalog lot is saved for that account
- **AND** the action does not create a bid, extend a schedule, reserve the lot, or determine an outcome

#### Scenario: Draft disclosure attempt
- **WHEN** a user attempts to save a draft ID or a missing ID
- **THEN** the API returns the same not-found status and error shape
- **AND** neither a watch entry nor private catalog fields are disclosed

### Requirement: Repeat-safe membership changes
Saving an already saved lot and removing an absent entry SHALL succeed without duplicate rows or errors. Each account/lot pair SHALL have at most one membership. Successful mutations SHALL reflect committed storage; retries after an uncertain result SHALL safely converge on the requested membership.

#### Scenario: Double save and concurrent requests
- **WHEN** two save requests for the same account and published lot overlap or a successful save is repeated
- **THEN** exactly one membership remains and each confirmed response reports saved
- **AND** repeat saving does not change the original saved time or reorder the list

#### Scenario: Repeat removal
- **WHEN** a user removes a saved entry twice or removes a lot not in their list
- **THEN** both confirmed requests report not saved and only that user's membership is affected
- **AND** removal reveals no information about draft or other-account membership

### Requirement: Persistent private listing
`/account/watchlist` SHALL display the authenticated account's committed saved lots after direct load, later login, and server restart. Entries SHALL use published catalog projections, newest-saved first with stable lot-ID tie breaking, and link to their actual lot details. Responses SHALL NOT expose another account, credentials, private notes, versions, or game secrets.

#### Scenario: Return after logout and restart
- **WHEN** an account saves multiple lots, logs out, and logs in after restart
- **THEN** its watchlist restores the same memberships in deterministic order with title, image/alt, opening price, planned times, and demo labeling
- **AND** expired planned times retain schedule wording without fabricated current bids or results

#### Scenario: Listed entry no longer has public content
- **WHEN** a stored membership cannot resolve to currently published content
- **THEN** listing omits that content without revealing a draft or substituting a fixture
- **AND** removal remains safe and does not affect other entries

### Requirement: Lot detail save interaction
Published lot detail SHALL offer a keyboard-operable save/remove control separate from the disabled bid control. Signed-in users SHALL see confirmed membership and explicit pending/failure states. Anonymous users SHALL receive a login link returning to that lot; login SHALL NOT silently perform a save.

#### Scenario: Anonymous visitor returns from login
- **WHEN** a visitor follows the watchlist login link from a published lot and completes authentication
- **THEN** the browser returns to the validated lot detail URL and the visitor can explicitly save it
- **AND** the bid control remains disabled and explained

#### Scenario: Failed or obsolete mutation
- **WHEN** saving/removal fails, authentication expires, or a response belongs to an obsolete page or account
- **THEN** the UI does not claim a successful change or expose the previous account's state
- **AND** it presents retry/login recovery and resolves uncertain outcomes by reloading committed membership

### Requirement: Recoverable bilingual watchlist page
Watchlist and save controls SHALL use the vintage design system, EN/VI interface copy, visible keyboard focus, announced pending/results, and usable mobile/200-percent-zoom layouts. Loading, signed-out, empty, backend-unavailable, and other failure states SHALL be distinct. Failed reads SHALL NOT be presented as an empty list.

#### Scenario: Empty and unavailable states
- **WHEN** a new authenticated account has no saved lots or the API fails
- **THEN** the former shows an empty-list explanation and browse-lots link while the latter shows recovery for the failed request
- **AND** successful removal updates the list and its count only after confirmation

#### Scenario: Mobile keyboard interaction
- **WHEN** a user saves or removes a long-titled lot at 320 CSS pixels or 200 percent zoom using the keyboard
- **THEN** all controls, status messages, links, and focus remain usable without page-level horizontal overflow
- **AND** switching EN/VI changes interface messages without altering saved catalog content

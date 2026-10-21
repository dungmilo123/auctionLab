# Spec Delta

## MODIFIED Requirements

### Requirement: Durable once-only terminal results
At or after closing, a lot SHALL receive exactly one immutable sold result for its highest accepted bid, or an unsold result when no bid exists. Results SHALL retain lot, closing instant, recording instant and winning bid/account/amount when sold. Sold closure SHALL atomically create one awaiting-payment order for that winning result; unsold closure SHALL create none. Closing SHALL NOT record payment, charge funds, extend a deadline or relist the lot.

#### Scenario: Competing accounts and closure
- **WHEN** A and B place valid increasing bids and B holds the highest accepted bid when the lot closes
- **THEN** the result identifies B's bid and exact winning amount consistently across public and private reads
- **AND** repeated closure cannot change the winner or create another result or order

#### Scenario: No accepted bid
- **WHEN** a published lot reaches closing without an accepted bid
- **THEN** it closes unsold with no winning bid, account or amount
- **AND** it creates no order and is not automatically reopened or relisted

#### Scenario: Worker delay or restart after deadline
- **WHEN** a nonterminal lot is already past deadline or the server restarts after downtime
- **THEN** new bids are refused immediately and presentation remains closing until a terminal result commits
- **AND** reconciliation uses durable accepted bids and original dates, surviving another restart without duplicate results

#### Scenario: Sold closure and order are atomic
- **WHEN** a new sold closure succeeds or fails before commit
- **THEN** its result and exactly one owner order commit together or neither commits
- **AND** retry preserves the highest accepted bid and creates no duplicate order

### Requirement: Private participation reads
`/api/account/bids` and its lot-detail reads SHALL require a current commerce session and expose only that owner's accepted participation. Each listed lot SHALL include the owner's latest accepted amount, public lot link, current price and server-confirmed leading/outbid/closing/won/lost state. Winning account identifiers SHALL remain private. Won participation SHALL include its owned order ID, awaiting-payment/paid state and checkout availability from committed order data.

#### Scenario: Leading, outbid and final participation
- **WHEN** A leads, B overtakes A, and B wins at close
- **THEN** A sees leading then outbid then lost, while B sees leading then won; closure pending is shown distinctly
- **AND** no failed submission appears in either account's participation list

#### Scenario: Private lot reconciliation and ordering
- **WHEN** an account reads its lot participation or the all-participation list
- **THEN** its lot read includes only its own accepted request IDs/bids or not-participating, and the list orders unique lots by latest personal bid time descending then lot ID
- **AND** the page offers leading, outbid, won and lost filters while All also shows closing lots

#### Scenario: Account isolation and login recovery
- **WHEN** another account, an expired session or an admin capability attempts to read an owner's bid participation
- **THEN** no other owner's data is returned, invalid sessions receive HTTP 401, and the page offers a safe login return
- **AND** logout/account switching/obsolete responses clear private state without replaying a bid after login

#### Scenario: Follow a won lot into checkout
- **WHEN** the winner reads won participation after order creation or simulated payment
- **THEN** that owner receives the stable order link and current awaiting-payment or paid state
- **AND** losing accounts and public snapshots receive no order identifier, payment receipt or checkout contact data

### Requirement: Accessible confirmed bid interaction
Lot bidding and `/account/bids` SHALL use EN/VI interface copy, exact money displays, labeled keyboard controls, announced results and the vintage design system. Mobile layouts and 200-percent zoom SHALL remain usable. Loading, signed-out, empty, unavailable, pending, refusal and unknown outcomes SHALL be distinct; success SHALL require server confirmation.

#### Scenario: Two-browser happy path
- **WHEN** two separate logged-in browsers compete on a published lot
- **THEN** each can deliberately submit a valid bid and inspect accepted history, current minimum and personal leading/outbid state
- **AND** the final winner sees won with a link to the owned single-order checkout or its paid confirmation

#### Scenario: Mobile keyboard and language
- **WHEN** bidding/history/account views are used at 320 or 375 CSS pixels, desktop or actual 200-percent browser zoom with keyboard navigation and either language
- **THEN** controls, money, long titles, errors, result messages and links remain usable without page-level horizontal overflow
- **AND** realtime updates preserve focus and countdown ticks do not repeatedly announce to assistive technology

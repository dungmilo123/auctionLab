# Spec Delta

## Purpose

Enable commerce participants to compete on independently timed vintage lots and receive durable, server-confirmed auction outcomes without inheriting classroom-game economics.

## ADDED Requirements

### Requirement: Independent published lot auctions
Each lot in a published event SHALL have at most one commerce auction using its immutable USD pricing and published opening/closing schedule. Overlapping events and simultaneously open lots SHALL be supported. Drafts SHALL NOT execute, and commerce auctions SHALL NOT change catalog publication, game records, budgets, truth values, or game revisions.

#### Scenario: Concurrent lots across events
- **WHEN** two published events overlap and several of their lots reach opening
- **THEN** each eligible lot accepts independent competition using its own price, increment, history and deadline
- **AND** bids and results on one lot do not change the state of another lot or the retained game

#### Scenario: Draft or unknown target
- **WHEN** a client requests an auction or submits a bid for a draft lot or missing lot
- **THEN** both targets return indistinguishable not-found responses
- **AND** no auction or bid is created for either target

### Requirement: Scheduled activation and adoption
Normal startup SHALL adopt previously published lots using their original schedules and reconcile overdue execution before accepting traffic. Newly published lots SHALL become eligible without restarting. Initialization SHALL be repeat-safe and SHALL NOT invent bids, rebase dates, edit publication, or run during offline maintenance.

#### Scenario: Upgrade with existing published lots
- **WHEN** the first bidding-enabled server starts with future, currently eligible and expired published schedules
- **THEN** they become scheduled, open and durably unsold respectively if no accepted bids exist
- **AND** recorded closure identifies the original deadline and actual result-recording time without fabricating historical participation

#### Scenario: Publish while serving
- **WHEN** an operator publishes a complete future-scheduled event while the server is running
- **THEN** its lots become eligible according to their published schedules without a server restart
- **AND** a brief initialization delay is identified as execution pending rather than a fabricated open state

### Requirement: Server-authoritative bid window
A new bid SHALL be eligible only when `opens_at <= server_time < closes_at`. Eligibility SHALL use server time sampled inside the serialized mutation after lock waiting, with session validity rechecked at that instant. Client timestamps, HTTP arrival time, countdowns and worker delays SHALL NOT extend eligibility.

#### Scenario: Exact boundaries
- **WHEN** a valid account bids immediately before opening, exactly at opening, immediately before closing, and exactly at closing
- **THEN** only the two submissions within the half-open interval are eligible for amount validation
- **AND** a delayed closure worker cannot accept a bid at or beyond closing

#### Scenario: Request waits behind another mutation
- **WHEN** a request arrives before closing but obtains mutation ownership after the deadline or after its session expires
- **THEN** it is refused using the time after waiting
- **AND** a bid validated before the deadline can commit afterward before the serialized closure without changing its acceptance instant

### Requirement: Exact ascending commerce amounts
Bids SHALL use integer USD cents in `amount_minor`, from zero through 1,000,000,000,000. The first minimum SHALL equal the opening price; later minima SHALL equal the highest accepted amount plus the fixed increment. Amounts at least the minimum SHALL be accepted without a multiple-of-increment restriction. Invalid arithmetic SHALL never wrap or round.

#### Scenario: Opening bid and jump bid
- **WHEN** opening price is 12550 cents with a 500-cent increment
- **THEN** the first bid of 12550 is eligible, the next minimum is 13050, and a bid of 14001 is eligible with next minimum 14501
- **AND** a below-minimum amount is refused without appending a bid

#### Scenario: Zero and invalid input
- **WHEN** a lot opens at zero or a request supplies fractional cents, a negative amount or an over-limit amount
- **THEN** a zero opening bid is valid, while the invalid inputs receive structured validation errors
- **AND** decimal form entry preserves cents exactly and missing values are not treated as zero

#### Scenario: Next minimum exceeds the range
- **WHEN** the highest accepted amount plus increment exceeds the maximum permitted amount
- **THEN** the next minimum is unavailable with an explicit amount-limit reason and further submissions are disabled/refused
- **AND** the existing leading bid remains eligible to win at the original closing time

### Requirement: Commerce identity and bid request protection
Bid submission SHALL derive its account solely from a current commerce session, require strict bounded JSON and the explicit account-request header, and reject foreign supplied origins. Both commerce roles SHALL bid only as themselves. Game tokens and catalog capability cookies SHALL NOT authorize bids; watchlisting SHALL NOT be required.

#### Scenario: Authenticated competition
- **WHEN** two valid commerce accounts submit their own bids with the required same-origin request boundary
- **THEN** accepted bids are attributed to those session owners
- **AND** caller-supplied account, role, deadline or winner fields are rejected

#### Scenario: Invalid session or request boundary
- **WHEN** a request has an expired/revoked/absent session, only a game token or admin capability, a foreign Origin, missing header, malformed JSON or unsupported fields
- **THEN** it is rejected with a structured no-store response and no accepted bid
- **AND** no cross-origin credential access is granted

### Requirement: Atomic ordered acceptance
Concurrent bids on a lot SHALL serialize against its latest committed state. An acceptance SHALL atomically append one immutable bid, advance the lot's sequence/revision, and update its count and leader before success is returned. A current leader SHALL NOT raise their own bid. Refusal SHALL NOT append history or change the leader.

#### Scenario: Competing minimum submissions
- **WHEN** two accounts concurrently submit the same current minimum on a lot with a positive increment
- **THEN** exactly one bid is accepted and the other receives the updated minimum conflict
- **AND** the accepted sequence, count, leader and revision agree in subsequent reads

#### Scenario: Current leader submits again
- **WHEN** the leading account submits a fresh request with a higher amount
- **THEN** the request is refused as already leading
- **AND** another account can compete under the current amount and deadline rules

### Requirement: Retry-safe accepted intents
Every submission SHALL carry a bounded request ID scoped to its account. Replaying an accepted ID with the same lot/amount SHALL return the original receipt without a second bid, even after closure; a different payload SHALL conflict. Unknown commit outcomes SHALL NOT be reported as accepted. Rejected requests SHALL NOT reserve an ID or appear as bids.

#### Scenario: Double submission and retry after restart
- **WHEN** identical requests overlap, or a committed request is retried after restart or closure with a valid session
- **THEN** there is one accepted bid and all successful receipts identify it
- **AND** reusing its ID for another lot or amount is refused without altering history

#### Scenario: Lost response or uncertain commit
- **WHEN** a submit response is lost or storage cannot confirm commit
- **THEN** the website shows an unknown outcome and reconciles the owner's accepted request IDs or explicitly retries the same intent with its same ID
- **AND** it never silently creates a new intent or claims acceptance from a local amount change

### Requirement: Durable once-only terminal results
At or after closing, a lot SHALL receive exactly one immutable sold result for its highest accepted bid, or an unsold result when no bid exists. Results SHALL retain lot, closing instant, recording instant and winning bid/account/amount when sold. Closing SHALL NOT create an order, charge funds, extend a deadline or relist the lot.

#### Scenario: Competing accounts and closure
- **WHEN** A and B place valid increasing bids and B holds the highest accepted bid when the lot closes
- **THEN** the result identifies B's bid and exact winning amount consistently across public and private reads
- **AND** repeated closure cannot change the winner or create another result

#### Scenario: No accepted bid
- **WHEN** a published lot reaches closing without an accepted bid
- **THEN** it closes unsold with no winning bid, account or amount
- **AND** it is not automatically reopened or relisted

#### Scenario: Worker delay or restart after deadline
- **WHEN** a nonterminal lot is already past deadline or the server restarts after downtime
- **THEN** new bids are refused immediately and presentation remains closing until a terminal result commits
- **AND** reconciliation uses durable accepted bids and original dates, surviving another restart without duplicate results

### Requirement: Private participation reads
`/api/account/bids` and its lot-detail reads SHALL require a current commerce session and expose only that owner's accepted participation. Each listed lot SHALL include the owner's latest accepted amount, public lot link, current price and server-confirmed leading/outbid/closing/won/lost state. Winning account identifiers SHALL remain private.

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

### Requirement: Accessible confirmed bid interaction
Lot bidding and `/account/bids` SHALL use EN/VI interface copy, exact money displays, labeled keyboard controls, announced results and the vintage design system. Mobile layouts and 200-percent zoom SHALL remain usable. Loading, signed-out, empty, unavailable, pending, refusal and unknown outcomes SHALL be distinct; success SHALL require server confirmation.

#### Scenario: Two-browser happy path
- **WHEN** two separate logged-in browsers compete on a published lot
- **THEN** each can deliberately submit a valid bid and inspect accepted history, current minimum and personal leading/outbid state
- **AND** the final winner sees won with checkout identified as unavailable

#### Scenario: Mobile keyboard and language
- **WHEN** bidding/history/account views are used at 320 or 375 CSS pixels, desktop or actual 200-percent browser zoom with keyboard navigation and either language
- **THEN** controls, money, long titles, errors, result messages and links remain usable without page-level horizontal overflow
- **AND** realtime updates preserve focus and countdown ticks do not repeatedly announce to assistive technology

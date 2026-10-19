# Spec Delta

## Purpose

Let visitors observe committed commerce auction changes promptly and recover a consistent view after disconnects without exposing private account or retained-game state.

## ADDED Requirements

### Requirement: Public coherent auction snapshots
Published lot auction reads SHALL expose one coherent committed snapshot with lot ID, per-lot revision, execution state, schedule, USD amounts, bid count, leader and recorded result. Before any accepted bid, current price SHALL be null and opening price distinct. Missing execution SHALL be pending, and expired nonterminal execution SHALL be closing rather than a fabricated result.

#### Scenario: First view and accepted bid
- **WHEN** an anonymous visitor loads an initialized published lot and another account successfully bids
- **THEN** its snapshots progress from zero bids/null current price to the accepted amount, count and matching leader/revision
- **AND** each response includes valid server time without exposing a mixed pre/post-commit aggregate

#### Scenario: Execution pending or close not committed
- **WHEN** a published lot lacks initialized execution or has reached its deadline without a terminal commit
- **THEN** the UI explains pending or closing, disables bid submission and does not announce sold, unsold or a personal win
- **AND** opening/closing countdowns never become authority to fabricate a result

### Requirement: Pseudonymous accepted history
Public history SHALL contain only committed accepted bid sequence, amount, acceptance time and a stable per-lot bidder alias. Latest history and cursor pages SHALL have at most 50 entries each, newest sequence first. Rejections SHALL NOT appear. Aliases SHALL NOT expose email, display name, account ID or a reusable cross-lot identifier.

#### Scenario: History grows beyond a snapshot page
- **WHEN** a lot has more than 50 accepted bids
- **THEN** its snapshot includes the newest 50 and identifies additional history, and a before-sequence cursor loads earlier bids without repeating entries
- **AND** accepted sequence determines order even when server timestamps match

#### Scenario: Same participant bids on several lots
- **WHEN** one account participates on two lots and another account participates on one
- **THEN** each lot assigns its own stable Bidder N aliases and the public winner uses the winning bid's alias and amount
- **AND** no account identity or cross-lot correlation key is included in the response

### Requirement: Read-only commerce streaming
A dedicated public commerce stream SHALL deliver an initial full auction snapshot and subsequent committed revisions, with periodic server-time heartbeats. Streams SHALL NOT accept bid commands, require a commerce login, mutate auctions, or connect to the retained game protocol. Healthy local two-browser rehearsal SHALL demonstrate bid/result visibility within two seconds.

#### Scenario: Watch a competing account
- **WHEN** an anonymous or signed-in visitor watches a lot while another account bids or its result commits
- **THEN** the visitor receives the updated full snapshot through the commerce stream within the measured local acceptance target
- **AND** no game socket, player token or game-role projection is used

#### Scenario: Read without mutation
- **WHEN** public snapshot, history or stream requests occur before initialization or after a deadline
- **THEN** they expose only committed data with honest pending/closing presentation
- **AND** those GET requests do not initialize, settle, accept bids or alter revisions

### Requirement: Monotonic recovery from reconnects
Clients SHALL recover by reading a fresh full snapshot on reconnect, refresh or focus restoration. Lower per-lot revisions SHALL NOT overwrite newer state; same-revision time/presentation refreshes SHALL remain possible. A dropped stream SHALL offer visible recovery and fallback reads. Expired data SHALL NOT remain an apparently current enabled bid form.

#### Scenario: Missed updates and stale response
- **WHEN** a browser disconnects through several bids and reconnects while an older HTTP read completes
- **THEN** the latest snapshot restores current history/count/leader/result and the older revision is discarded
- **AND** a prior accepted bid remains accepted even if its HTTP receipt arrives later than a newer snapshot

#### Scenario: Stream unavailable or data stale
- **WHEN** streaming fails or no successful snapshot/time heartbeat arrives for ten seconds
- **THEN** the page labels reconnecting/stale state, performs fallback reads and disables submission once stale
- **AND** a successful fresh read restores eligibility only after current account and auction checks

### Requirement: Bounded stream lifecycle
Streaming SHALL bound concurrent connections, history payloads and slow-client writes. Disconnect, navigation and server shutdown SHALL release stream resources promptly without retaining database transactions or writer ownership. Stream capacity or transport failure SHALL NOT block bid acceptance, closure or other visitors.

#### Scenario: Slow client and capacity
- **WHEN** a client stops reading or the configured stream limit is reached
- **THEN** the slow stream terminates within its write bound or the excess connection receives retry guidance
- **AND** other bidders and due closures continue without waiting for that client

#### Scenario: Leave the lot or stop the server
- **WHEN** the page unmounts, its stream disconnects or normal shutdown begins
- **THEN** its stream/poll resources are cancelled and shutdown completes before storage is closed
- **AND** no blocked stream read/write keeps the service alive indefinitely

### Requirement: Separate private participation freshness
Personal leading/outbid/closing/won/lost state SHALL come from current session-owned reads, never from matching public aliases to profile fields. Visible account participation SHALL refresh after bid completion, relevant public updates and focus, with periodic recovery. Account changes and invalid sessions SHALL clear private state and pending intent.

#### Scenario: Public stream continues after logout
- **WHEN** a browser logs out or changes accounts while watching a lot
- **THEN** public auction updates remain available, but previous private bids, request IDs and participation labels disappear
- **AND** the new account receives only its own freshly validated participation and no automatic submission

#### Scenario: Identity service fails
- **WHEN** the public stream remains healthy but private participation cannot be loaded
- **THEN** price/history remain inspectable and the page identifies private state as unavailable
- **AND** it does not infer signed-out status, a win, or permission to submit from public data alone

### Requirement: Explicit realtime privacy boundary
Public snapshots, history, streams and failures SHALL use an explicit safe field set excluding account IDs, credentials, request IDs, private notes, catalog versions, capability keys and game secrets. Draft and nonexistent lot IDs SHALL remain indistinguishable, and all auction responses SHALL prevent caching of stale execution state.

#### Scenario: Wire inspection
- **WHEN** published content and bidder records contain distinctive private values
- **THEN** public snapshot/history/stream/error payloads contain none of those values and private request IDs remain owner-only
- **AND** public terminal results disclose only the winning public bid reference, alias and amount

#### Scenario: Unpublished stream target
- **WHEN** a client requests snapshot, history or streaming for a draft or missing lot
- **THEN** both receive the same no-store not-found response without opening a stream
- **AND** the request neither exposes draft existence nor mutates storage

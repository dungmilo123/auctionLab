# auction-event-publishing Specification

## Purpose

Let operators prepare and publish coherent auction events with ordered lots and durable planned times, providing configuration for the later timed-auction lifecycle.

## Requirements

### Requirement: Draft event content
An event SHALL have a stable ID, unique editable draft slug, title, theme, description, cover image/alternative text, admin notes, ordered lot membership, and planned schedule. Creating a draft SHALL require only a title; missing publication fields SHALL remain editable. Draft saves SHALL reject invalid supplied data and preserve the prior record on failure.

#### Scenario: Prepare an event incrementally
- **WHEN** an operator creates a titled draft and later adds its theme, description, cover, notes, and schedule
- **THEN** each successful save is durable under the same event ID
- **AND** the draft remains unavailable through public catalog endpoints until publication

### Requirement: Exclusive lot assignment and ordering
A lot SHALL belong to at most one event or remain unassigned. Operators SHALL assign, detach, and reorder draft lots in draft events. Membership replacement SHALL reject duplicates, unknown lots, and lots belonging to another event; reassignment SHALL require detaching first. Published membership SHALL be immutable.

#### Scenario: Set the running order
- **WHEN** an operator assigns three unassigned drafts and orders them C, A, B
- **THEN** that exact order survives saving and restart
- **AND** the lot closing schedule follows C, A, B

#### Scenario: Invalid or conflicting assignment
- **WHEN** an operator submits a duplicate lot, a missing lot, or a lot already assigned elsewhere
- **THEN** the entire membership update is rejected
- **AND** neither event nor any lot is partly reassigned

#### Scenario: Move a draft lot
- **WHEN** an operator detaches a lot from one draft event and assigns it to another
- **THEN** it belongs only to the destination event and receives that event's derived schedule
- **AND** trying to detach or reassign a published lot fails

### Requirement: Common opening and staggered closing schedule
An event SHALL define `opens_at`, `first_closes_at`, and a positive closing interval in integer milliseconds. Every assigned lot SHALL share `opens_at`; the lot at zero-based position k SHALL close at `first_closes_at + k * close_interval_ms`. The first close SHALL follow opening. Overlapping events SHALL be allowed. Invalid or overflowing schedules SHALL be rejected.

#### Scenario: Calculate successive closes
- **WHEN** three ordered lots have an opening at 10:00 UTC, first close at 11:00 UTC, and a 60-second interval
- **THEN** all open at 10:00 and the planned closes are 11:00, 11:01, and 11:02 UTC respectively
- **AND** the event's planned end is the last lot's close

#### Scenario: Reorder or change a draft schedule
- **WHEN** an operator reorders draft lots or changes a draft event's timing
- **THEN** all planned lot closes are recalculated consistently and shown before publication
- **AND** local time displays preserve the same underlying UTC instants after saving

#### Scenario: Invalid timing
- **WHEN** the first close is at or before opening, the interval is zero/negative, a timestamp is invalid, or the last close exceeds the supported range
- **THEN** the schedule is rejected with an identifiable validation error
- **AND** no part of the prior schedule changes

### Requirement: Complete publication validation
Publication SHALL require complete event content, at least one lot, and complete lot category, description, condition, gallery, starting price, positive increment, and fulfillment terms. Delivery SHALL require a nonnegative fee and instructions; pickup SHALL require instructions; at least one option SHALL be enabled. Publication SHALL require a valid future opening time and readable referenced media, and return field/lot-specific errors.

#### Scenario: Incomplete event cannot publish
- **WHEN** an event is missing its cover, contains an incomplete lot, or offers delivery without its disclosed fee
- **THEN** publication fails with errors identifying the event or affected lot fields
- **AND** the operator's saved draft remains available for correction

#### Scenario: No slots or expired opening
- **WHEN** an event has no lots or its planned opening is no longer in the future at publication
- **THEN** publication is refused without changing public visibility
- **AND** the operator can edit the draft schedule and retry

### Requirement: Atomic immutable publication
Publishing SHALL atomically mark an event and all its assigned lots published, freeze their content/order/planned schedule, and make their public projections available. A failed publication SHALL expose none of them. Repeating publication of the same published event SHALL return its committed result without duplicating records. Published event or lot edits SHALL be rejected in this change.

#### Scenario: Publish a complete event
- **WHEN** an operator publishes a valid draft containing six complete lots
- **THEN** public reads expose the event and all six lots with matching membership and planned dates
- **AND** restart preserves that exact published aggregate

#### Scenario: Failure during publication
- **WHEN** publication fails validation or storage commit
- **THEN** neither the event nor any of its lots becomes publicly visible
- **AND** the stored draft content is retained

#### Scenario: Repeated publication and later editing
- **WHEN** an operator repeats a successful publication and then attempts to edit its event, lot content, slug, pricing, order, or schedule
- **THEN** publication returns the existing result without creating duplicates
- **AND** every attempted edit fails with a published-record conflict

### Requirement: Consistent concurrent preparation
Catalog updates SHALL reject stale record versions rather than silently overwrite newer preparation. Publication SHALL validate the latest committed aggregate. Concurrent membership, lot, or schedule changes SHALL NOT bypass completeness checks, change a published aggregate, or produce partial publication.

#### Scenario: Two editors save the same draft
- **WHEN** two editors load the same version and save different changes
- **THEN** the first committed update advances the version and the stale update receives a conflict
- **AND** the stale editor can reload without losing the committed content

#### Scenario: Edit races with publish
- **WHEN** one request publishes while another changes a lot or event membership
- **THEN** one consistent committed ordering determines the result
- **AND** the event is either a valid published aggregate or remains a draft with a retryable conflict

#### Scenario: Concurrent pooled database connections
- **WHEN** edit, membership, and publish requests execute concurrently through different PostgreSQL connections
- **THEN** the same stale-version, exclusive-assignment, completeness, and immutable-publication rules hold
- **AND** correctness does not depend on a single open connection or an in-process catalog mutex

### Requirement: Publication is separate from auction execution
Passing planned opening or closing times SHALL NOT create bids, execute a game round, determine a winner, or settle a catalog lot in this change. Public responses SHALL distinguish editorial publication from auction execution, exposing planned dates without claiming live, sold, ended, or winning states.

#### Scenario: Time passes after publication
- **WHEN** the server restarts after a published event's planned closing time
- **THEN** its content, publication state, and planned dates remain unchanged
- **AND** no catalog auction start, bid, winner, order, or settlement is created by those dates; retained legacy-game restart behavior remains separate

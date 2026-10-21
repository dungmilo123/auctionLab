# auction-analytics Specification

## Purpose

Give authorized operators consistent auction participation and outcome summaries with explicit denominators, demo provenance and simulated payment values.

## Requirements

### Requirement: Private aggregate dashboard
`/admin/analytics` and `GET /api/admin/analytics` SHALL require the existing admin capability independently of commerce roles. The API SHALL return only aggregate metrics, public event identity and filter/coverage metadata with no-store responses. Unauthorized requests SHALL reveal no metrics or private existence information.

#### Scenario: Capability and account separation
- **WHEN** a bidder or commerce-admin session requests analytics without the capability
- **THEN** the Go document request reaches the existing gate and the API returns sanitized 401
- **AND** an authorized operator can read aggregates without signing into or impersonating a buyer

#### Scenario: Aggregate projection
- **WHEN** an authorized operator loads dashboard data
- **THEN** the response contains no account identifiers, names, emails, watchlist membership, order/receipt identifiers, addresses, shipment details, credentials, raw interactions or game secrets
- **AND** every success and error response is no-store

### Requirement: Defined lifetime event cohort
Analytics SHALL include only published events and their published lots, using lifetime committed facts as of one response snapshot. Filters SHALL combine `event=all|<published-slug>` and `dataset=all|demo|non-demo`, defaulting to all. A lot SHALL be demo when it or its event is demo; actor demo flags SHALL remain a separate participation breakdown.

#### Scenario: Shareable selections
- **WHEN** the operator reloads `/admin/analytics?event=analog-memories&dataset=demo` or uses Back/Forward
- **THEN** controls and metrics reflect the same event and lot cohort
- **AND** unknown, duplicate, empty or malformed API parameters return 422; a missing/draft event slug returns the same 404

#### Scenario: Mixed provenance
- **WHEN** a demo account bids on a non-demo lot or a non-demo lot belongs to a demo event
- **THEN** the former belongs to the non-demo lot cohort with demo participation identified separately and the latter belongs to the demo lot cohort
- **AND** every paid-value metric remains explicitly simulated, including non-demo content

### Requirement: Participation metrics use accepted bids
Bid count SHALL count accepted commerce bid rows once. Bidder count SHALL count distinct commerce accounts with accepted bids in the selected cohort, rather than registered users, visits or aliases. Per-event bidder counts SHALL deduplicate within each event; overall bidder count SHALL deduplicate across all selected events.

#### Scenario: Repeat bidder and retry
- **WHEN** one account places two accepted bids in event A, one in event B, then retries an accepted request
- **THEN** A has two bids and one bidder, B has one bid and one bidder, and the combined cohort has three bids and one bidder
- **AND** rejected bids, retries, views and watchlist additions do not increase these metrics

#### Scenario: Demo actor counts
- **WHEN** both demo and ordinary accounts place accepted bids in the cohort
- **THEN** demo and non-demo distinct bidder counts and bid counts are shown separately using the actor flags
- **AND** those actor counts never expose an individual account

### Requirement: Sell-through uses committed terminal outcomes
The dashboard SHALL expose published lot count, sold count, unsold count and pending-result count. Sell-through SHALL be sold divided by sold plus unsold, with null when no terminal results exist. Pending-result count SHALL include every cohort lot without a committed result, regardless of its schedule or initialization state.

#### Scenario: Partial closure
- **WHEN** a cohort contains two sold lots, one unsold lot and three lots without committed results
- **THEN** lot count is six, terminal count is three, pending-result count is three and sell-through is two thirds
- **AND** the dashboard displays numerator/denominator and identifies rounding as presentation only

#### Scenario: No terminal results
- **WHEN** no cohort lot has a committed sold or unsold result
- **THEN** sell-through is unavailable with denominator zero rather than a misleading zero percent
- **AND** elapsed schedules alone never imply unsold status

### Requirement: Winning and simulated paid values stay distinct
Winning value SHALL sum committed sold-result winning amounts, including unpaid wins. Simulated paid value SHALL sum one stored simulated receipt per cohort order, including its selected delivery fee. The dashboard SHALL separately expose paid winning value, paid delivery fees, paid-order count and unpaid sold-order count; pending orders SHALL add no paid value.

#### Scenario: Paid shipping and unpaid win
- **WHEN** two sold lots have winning amounts 10000 and 20000 cents and only the first is paid with a 1500-cent shipping fee
- **THEN** winning value is 30000, paid winning value is 10000, paid fees are 1500 and simulated paid value is 11500 cents
- **AND** paid-order count and unpaid sold-order count are each one; payment replay and fulfillment advancement change no totals

#### Scenario: Paid amount consistency
- **WHEN** required receipt/order amounts or sold-order bindings are inconsistent
- **THEN** the request returns sanitized unavailable rather than omitting bad records or presenting a partial successful total
- **AND** a zero winning price or zero delivery fee remains an explicit valid zero

### Requirement: Consistent comparisons and exact totals
Summary and per-event rows SHALL use one committed snapshot, identical formulas and filters, and stable event-slug ordering. Integer counts and USD-cent totals SHALL be transmitted as nonnegative decimal strings and formatted without floating-point loss. Response metadata SHALL identify USD, snapshot time, demo cohort and interaction coverage.

#### Scenario: Concurrent commits and large totals
- **WHEN** bids or payments commit during a read, or aggregate cents exceed JavaScript's safe integer range
- **THEN** all rows and the summary reflect one coherent committed snapshot and displayed totals remain exact
- **AND** sums of additive event counts/totals match the summary while distinct bidder counts are explicitly non-additive

### Requirement: Honest dashboard states and interaction coverage
The dashboard SHALL provide EN/VI metric definitions, filters, loading, successful empty, access-required, unavailable and retry states. Interaction counts SHALL separate observed records from reconstructed history and watchlist snapshots, report collection start and identify views as recorded page observations rather than unique people. Failed reads SHALL NOT become successful zeroes.

#### Scenario: Empty, unavailable and accessible comparison
- **WHEN** the cohort is empty, the API fails, or the operator navigates by keyboard at mobile widths or actual 200 percent browser zoom
- **THEN** successful emptiness and failure remain distinct, retry preserves filters and comparison data remains readable with labeled controls and table headers
- **AND** EN/VI, reduced motion and text alternatives do not require color or animation to interpret metrics

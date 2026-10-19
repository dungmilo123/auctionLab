# Spec Delta

## MODIFIED Requirements

### Requirement: Publication is separate from auction execution
Publication SHALL expose immutable editorial content and schedule without accepting bids or determining a winner. A separate commerce lifecycle SHALL adopt published schedules and execute timed auctions, including overdue recovery. Editorial public catalog responses SHALL remain distinct from execution responses. Neither publication nor commerce execution SHALL run a legacy game round or create an order/payment.

#### Scenario: Time passes after publication
- **WHEN** the server restarts after a published event's planned closing time
- **THEN** its content, publication state and planned dates remain unchanged while the commerce lifecycle reconciles durable sold/unsold results using accepted commerce bids
- **AND** absent bids produce unsold rather than fabricated participation; retained legacy-game restart behavior remains separate

#### Scenario: Publication makes future execution eligible
- **WHEN** an operator publishes an event with a valid future opening
- **THEN** the existing atomic publication result exposes all its immutable public content and schedules
- **AND** a separate lifecycle adopts those lots without needing a restart, without making the publication request itself accept a bid or determine a result

#### Scenario: Catalog reads remain editorial
- **WHEN** a published lot is scheduled, open or terminal in the commerce service
- **THEN** its catalog DTO continues to report editorial published state, opening price and immutable schedule
- **AND** current price, execution state and results come from the separate auction response and cannot alter the catalog record

# Spec Delta

## MODIFIED Requirements

### Requirement: Published lot membership
Only a lot in a published event SHALL be addable to a watchlist. Draft and nonexistent targets SHALL return indistinguishable HTTP 404 responses. A saved lot SHALL remain saved when its window elapses; saving or schedule phase alone SHALL NOT label it sold, won or bid on. Execution and personal participation SHALL come only from the corresponding auction service.

#### Scenario: Save before or after the planned window
- **WHEN** an authenticated user saves an upcoming, within-window, or elapsed published lot
- **THEN** the same stable catalog lot is saved for that account
- **AND** the action does not create a bid, extend a schedule, reserve the lot, or determine an outcome

#### Scenario: Draft disclosure attempt
- **WHEN** a user attempts to save a draft ID or a missing ID
- **THEN** the API returns the same not-found status and error shape
- **AND** neither a watch entry nor private catalog fields are disclosed

### Requirement: Lot detail save interaction
Published lot detail SHALL offer a keyboard-operable save/remove control separate from the bid control. Signed-in users SHALL see confirmed membership and explicit pending/failure states. Anonymous users SHALL receive a login link returning to that lot; login SHALL NOT silently perform a save.

#### Scenario: Anonymous visitor returns from login
- **WHEN** a visitor follows the watchlist login link from a published lot and completes authentication
- **THEN** the browser returns to the validated lot detail URL and the visitor can explicitly save it
- **AND** authentication does not replay a bid; the separate bid control follows current commerce auction eligibility

#### Scenario: Failed or obsolete mutation
- **WHEN** saving/removal fails, authentication expires, or a response belongs to an obsolete page or account
- **THEN** the UI does not claim a successful change or expose the previous account's state
- **AND** it presents retry/login recovery and resolves uncertain outcomes by reloading committed membership

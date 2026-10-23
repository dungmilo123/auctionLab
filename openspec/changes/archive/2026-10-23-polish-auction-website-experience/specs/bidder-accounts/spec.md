# Spec Delta

## MODIFIED Requirements

### Requirement: Explicit repeat-safe demo accounts
An operator-only maintenance mode SHALL provision two provenance-marked demo bidder accounts and one admin account using supplied credentials. It SHALL be explicit, transactional, repeat-safe, and separate from normal boot and catalog seeding. It SHALL neither expose credentials nor overwrite existing accounts, passwords, roles, sessions, or watchlists. Demo status SHALL remain durable profile metadata without requiring a visible demo badge in buyer account pages.

#### Scenario: Seed and repeat
- **WHEN** the operator provisions accounts on an eligible schema and repeats the command
- **THEN** the intended accounts can log in, retain stable identities and demo provenance flags, and the repeat reports already applied
- **AND** no HTTP/tunnel/game service starts and no password is printed or shipped in frontend assets

#### Scenario: Collision or incomplete input
- **WHEN** required credentials are absent, an unowned account already uses a seed email, or another schema writer is active
- **THEN** provisioning fails clearly without partial accounts or a completion marker
- **AND** normal startup never installs default credentials

## ADDED Requirements

### Requirement: Focused account presentation
Account overview, registration, login, watchlist, bids and wins SHALL use coherent forms, icon-supported navigation and task-specific guidance without demo badges or capability inventories. Role display SHALL not imply catalog authority. Profile editing SHALL remain limited to the display name and account recovery SHALL not advertise unimplemented password-reset or email-verification actions.

#### Scenario: Profile and authentication use
- **WHEN** a user signs in and edits their profile in either language
- **THEN** email remains read-only, editable fields and errors have associated labels, and the saved value appears only after confirmed success
- **AND** native safe-return navigation, logout, pending controls and private-state clearing retain their existing behavior

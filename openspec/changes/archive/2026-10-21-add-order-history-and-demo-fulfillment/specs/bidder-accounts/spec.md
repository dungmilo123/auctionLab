# Spec Delta

## MODIFIED Requirements

### Requirement: Bidder and admin authorization boundary
Account roles SHALL be server-owned bidder or admin values. Public registration SHALL never grant admin. Account sessions SHALL NOT replace the existing admin document capability or independently authorize private catalog APIs. Possession of an admin capability SHALL NOT create an account session or authorize bidder-private profile, watchlist, participation or owner-order APIs. Dedicated admin order APIs SHALL permit only operational purchase/contact/receipt inspection and fulfillment updates; they SHALL NOT authorize buyer payment or impersonation.

#### Scenario: Bidder attempts catalog administration
- **WHEN** an authenticated bidder without an admin capability requests admin documents or any private catalog read, preview, or mutation
- **THEN** the existing document gate or JSON HTTP 401 applies respectively
- **AND** neither a client role claim nor an account session grants catalog authority

#### Scenario: Account admin and capability remain distinct
- **WHEN** a provisioned admin account logs in without the catalog capability
- **THEN** its self-profile reports the server-owned admin role but catalog APIs remain inaccessible
- **AND** entering the existing capability gate retains its prior operator behavior without converting that capability into bidder identity

#### Scenario: Narrow operational order exception
- **WHEN** a capability holder inspects another buyer's order through the dedicated admin API
- **THEN** order-processing details are available under that capability without selecting a commerce account
- **AND** the same capability grants no access to that buyer's self-profile, watchlist, bid participation or account order routes

# Spec Delta

## MODIFIED Requirements

### Requirement: Honest management and development boundaries
Catalog pages SHALL identify working content management and available commerce accounts/watchlists while keeping bidding and purchases unavailable. Bid/order pages SHALL retain preview status. A development admin page without a capability SHALL show an access-required state linking to the existing Go gate, preserve the HttpOnly credential boundary, and never expose draft data or simulate a successful save.

#### Scenario: Frontend-only development
- **WHEN** an operator opens a catalog admin page on the development frontend without a reachable authorized API
- **THEN** the page explains that access or the application server is required
- **AND** it does not fall back to mutable sample data or request a secret through a frontend field

#### Scenario: Return to public previews
- **WHEN** an operator saves or publishes content and visits the storefront, account, or order pages
- **THEN** discovery remains public, account/profile/watchlist pages use their independent commerce session, and bid/checkout/order activity remains explicitly unavailable
- **AND** working catalog management neither signs an account in nor creates watch entries, bids, checkout, or order processing

#### Scenario: Account sessions do not authorize catalog management
- **WHEN** a bidder or admin account session is present without a valid catalog capability
- **THEN** management documents and every private catalog read, preview, mutation, and unknown private path retain the existing gate/JSON-401 behavior
- **AND** acquiring the catalog capability does not reveal or select another person's commerce account

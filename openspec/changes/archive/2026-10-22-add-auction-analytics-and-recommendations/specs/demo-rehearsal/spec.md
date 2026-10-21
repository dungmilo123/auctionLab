# Spec Delta

## MODIFIED Requirements

### Requirement: Atomic documented reset write scope
Reset SHALL clear the target's sessions, accounts/watchlists, seed markers, catalog content, commerce auctions/bids/results, orders, simulated receipts, fulfillment history and interaction/collection-coverage records, plus retained game rehearsal data. It SHALL preserve schema migrations, the disposable marker and admin capability configuration. Clear/reseed and fresh collection coverage SHALL commit atomically; no cross-schema cascade or external media deletion SHALL occur.

#### Scenario: Reset failure and retry
- **WHEN** clear/reseed fails before commit because of validation, collision, dependency or storage failure
- **THEN** the previous rehearsal records and credentials remain intact
- **AND** failure or unknown commit is not reported as a confirmed fresh rehearsal; the operator inspects durable environment generation before retry

#### Scenario: Other data remains protected
- **WHEN** a reset succeeds while another schema contains operational purchases and an images directory contains media
- **THEN** other-schema records and all media files are unchanged
- **AND** old demo session cookies cease to authenticate because the target's prior sessions/accounts were removed

#### Scenario: New analytics rehearsal
- **WHEN** a completed marked rehearsal with recorded views/actions is explicitly reset
- **THEN** its prior interaction history is removed, collection coverage starts anew and new draft content contributes no published analytics or invented activity
- **AND** an explicit publish/view/watch/bid/pay journey produces metrics only for the new generation; normal boot and retained game reset preserve existing interaction history

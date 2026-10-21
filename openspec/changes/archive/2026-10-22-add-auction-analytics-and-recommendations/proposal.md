# Proposal

## Why

The auction-to-fulfillment journey now persists real commerce facts, but operators cannot compare participation and outcomes across events, and visitors have no related-lot discovery. Roadmap Change 8 adds measurable, explainable discovery after that journey, while keeping simulated payments and demo data explicit.

## What Changes

- Add capability-protected `/admin/analytics` with bidder/bid counts, committed sold/unsold counts, sell-through, winning-price totals, simulated paid totals and per-event comparison. Define every denominator, attribution rule and demo filter; distinguish unpaid wins and shipping fees from simulated payment value.
- Add up to six rule-based recommendations on home and lot detail: category/era similarity, owner-watchlist preferences and deterministic cold-start fallback. Return only eligible published lots, explain each suggestion and keep anonymous discovery independent of accounts.
- Record anonymous lot-detail views through a bounded first-party endpoint and watchlist additions, accepted bids and confirmed simulated payments through their authoritative server transactions. Preserve deduplication, provenance and collection coverage; historical facts never imply historical views.
- Add migration 6 without changing migrations 1–5; extend safe demo reset and SQLite-import occupancy checks for the new records. Use existing PostgreSQL, writer ownership, session and capability boundaries.
- Update EN/VI navigation, notices and recovery states for the available features; retain the separate optional ML extension and all earlier unresolved accessibility acceptance.

Out of scope: trained models, collaborative filtering, effectiveness claims, external analytics services, visitor fingerprinting, real payments, public dashboards, arbitrary date-range reporting, exports, funnels/conversion attribution, notifications and production warehouse infrastructure.

## Capabilities

### New Capabilities

- `auction-analytics`: Private, precisely defined aggregate metrics and consistent comparisons across published auction events.
- `lot-recommendations`: Explainable rule-based related lots, private watchlist preferences and deterministic public fallback.
- `commerce-interactions`: Minimal first-party interaction records, authoritative mutation capture, deduplication and honest historical coverage.

### Modified Capabilities

- `auction-discovery`: Add independent recommendation sections and optional view recording; replace future-feature notices while preserving public content and transaction boundaries.
- `website-navigation`: Recognize `/admin/analytics`, preserve document/API authorization and describe working analytics and rule-based recommendations honestly.
- `postgresql-persistence`: Add interaction migration/coverage, preserve existing state and extend import-occupancy protection.
- `demo-rehearsal`: Include interaction/coverage records in the explicitly bounded atomic reset and replay drill.

## Impact

Backend: new analytics/recommendation/interaction services and APIs; `internal/store` repositories and migration 6; transactional hooks in watchlist, bid and payment writes; explicit route wiring in `cmd/server/main.go`. Frontend: admin analytics, home/lot recommendations, view submission, API decoders, routes, labels and existing card/layout components. Tests use owned real-PostgreSQL schemas, focused frontend checks and Vite/embedded rehearsals. Operator docs include metric definitions, collection limits, schema upgrade and reset scope. No new external service or runtime dependency is required. This package authorizes planning only; archived Change 4–7 acceptance gaps remain visible.

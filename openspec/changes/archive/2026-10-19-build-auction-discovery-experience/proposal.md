# Proposal

## Why

Change 2 persists and publishes vintage auction content, but every public website destination still renders illustrative fixtures and generic detail pages. Change 3 connects those destinations to the published catalog so visitors can discover actual saved records from the homepage through lot details on desktop and mobile.

## What Changes

- Replace public fixtures at `/`, `/auctions`, `/auctions/:slug`, `/lots`, and `/lots/:slug` with anonymous reads of the existing public catalog APIs, including genuine missing-record states.
- Add featured events, lots nearest their planned close, category entry links, event schedules and ordered lot lists, and URL-backed lot search, category/schedule filters, and sorting.
- Present an ordered accessible gallery, complete lot descriptions/condition/specifications/supplied story, USD opening price and increment, planned times, and all enabled delivery/pickup terms and fees.
- Distinguish publication from schedule phases: “Upcoming schedule,” “Within planned window,” and “Planned window elapsed.” Add public response clock metadata for consistent phase presentation; these labels never assert auction execution or results.
- Provide loading, empty, no-results, not-found, backend-unavailable and recoverable error views, with EN/VI interface copy and local media fallbacks. Label stored demos accurately without treating every stored record as an illustrative placeholder.
- Keep bid controls disabled with an explanation. Accounts/watchlists (Change 4), auction execution/current bids/history/results (Change 5), checkout/orders, analytics, and ML remain outside scope. Preserve the retained game and admin capability boundary.

## Capabilities

### New Capabilities

- `auction-discovery`: Public homepage, event browsing, searchable lot discovery, rich lot detail, planned-schedule presentation, and recovery states using published content.

### Modified Capabilities

- `website-navigation`: Public detail routes resolve stored slug records, direct links restore discovery queries, and public discovery replaces fixture previews while account/order previews remain honest.
- `vintage-catalog`: Add `meta.server_time` in UTC Unix milliseconds to successful public catalog responses while preserving the published-only `data` DTOs and existing privacy/error contracts.

## Impact

- Frontend: `web/src/routes/storefront/`, `App.tsx`, `api/`, `components/site/Presentation.tsx`, `layouts/SiteLayout.tsx`, discovery helpers/copy/tests, and shared CSS. Reuse the native document-link router, local assets and design tokens.
- Backend: public response wrapping in `internal/api/catalog.go`, the injected catalog clock, and focused HTTP regression coverage. No database migration, publication/lifecycle mutation, new service, search infrastructure, or runtime dependency is required.
- Update active operator/frontend documentation and capture implementation acceptance evidence using disposable PostgreSQL schemas. Existing public API `data` consumers remain compatible; unknown slugs intentionally cease showing generic preview content.

# Proposal

## Why

AuctionLab currently presents a classroom auction game, while `ROADMAP.md` defines a vintage auction website with storefront, account, and administration journeys. Change 1 establishes the shared visual language and URL structure that later catalog, account, bidding, and order changes can build on without carrying the game's navigation or presentation assumptions forward.

## What Changes

- Establish the **Collected Stories** direction under the AuctionLab brand: ivory surfaces, charcoal text, burgundy actions, restrained olive/brass accents, serif headings, sans-serif body copy, and accessible responsive design tokens.
- Provide shared navigation, footer, buttons, form fields, lot cards, and status badges, including keyboard focus, disabled/error states, and mobile layouts.
- Add storefront, account, and admin layouts with explicitly labeled preview pages at the agreed routes. Preview examples demonstrate components; they do not represent published lots, user accounts, orders, or working transactions.
- Introduce explicit route matching, ordinary navigable links, direct-load/refresh support, and a not-found page with a recovery link.
- Bundle the selected fonts and illustrative assets with the application, including license/source records and local fallback behavior.
- **BREAKING**: Replace the player game at `/` and game control room at `/admin` with website shells; retire `/screen` and its descendants to the not-found view. Unknown URLs no longer enter the player game. No legacy game route alias is added.
- Preserve the existing server admin capability gate while styling its entry page consistently. Account authentication and new administration actions arrive in later changes.

## Capabilities

### New Capabilities

- `vintage-design-system`: Consistent accessible visual tokens and shared presentation components with locally served fonts and assets.
- `website-navigation`: Explicit website routes, context-appropriate layouts, preview boundaries, deep links, not-found recovery, and legacy route retirement.

### Modified Capabilities

None. The OpenSpec capability inventory is empty; the existing game implementation is recorded as the migration baseline in the design.

## Impact

- Frontend: `web/src/App.tsx`, `web/src/lib/route.ts`, `web/src/index.css`, shared primitives, new layout/page components, language copy, local assets, and document metadata. Existing React/Vite/TypeScript and styling dependencies suffice; no router or state-library dependency is proposed.
- Hosting: verify and narrowly adapt `internal/api/static.go`, `internal/api/admin_gate.go`, and `web/vite.config.ts` so nested URLs, protected admin documents, and missing asset responses behave consistently. Preserve the frontend-first embedded Go build.
- Compatibility: existing game URLs intentionally change. The engine, database schema/data, WebSocket protocol, and game tokens remain available for later evaluation but are not consumed by website previews. No data migration or deletion is included.
- Scope exclusions: catalog persistence/seed data, search/filter functionality, sign-in/watchlists, live bids/countdowns, pricing/currency contracts, checkout, orders, analytics, recommendations, and a working projector experience. These belong to later roadmap changes.
- Delivery is a responsive, navigable website foundation. It is not completion of any commerce feature.

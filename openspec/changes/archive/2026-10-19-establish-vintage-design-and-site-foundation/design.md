# Design

## Context

See `proposal.md` for motivation and scope. `ROADMAP.md` Change 1 is the authority for the new website direction; the earlier classroom-game constraints in project guidance describe the existing baseline where they conflict with that direction.

Read-only inspection found:

| Area | Observed baseline | Consequence |
|---|---|---|
| `web/src/App.tsx`, `web/src/lib/route.ts` | A pathname switch mounts player, admin, or screen; every unknown URL becomes player; no router/history library | Replace the selector and mounted views, with explicit misses |
| `web/src/index.css`, `components/ui/*` | Orange/cream neobrutalist tokens, thick borders, hard shadows, system fonts, projector/podium styles | Tokens alone cannot restyle the website; update shared primitive recipes too |
| `web/package.json` | React 19, Vite 8, TypeScript 6, Tailwind 4, Radix, oxlint, Vitest; no DOM testing harness | Reuse installed dependencies and existing pure unit tests; verify browser behavior directly |
| `web/src/lib/i18n.ts` | English/Vietnamese provider, persisted language, existing game copy | Retain language selection without importing game/session state |
| `internal/api/static.go` | All non-file URLs receive the SPA; missing `/assets/*` returns 404; HTML is no-cache and hashed assets immutable | Existing document deep links mostly work; add the local public-asset namespace to missing-file handling |
| `internal/api/admin_gate.go`, `cmd/server/main.go` | Server capability gate covers `/admin` and `/admin/`; valid keys redirect to literal `/admin`; entry HTML has inline game styling | Preserve authorization and redirect behavior; restyle the standalone gate |
| `web/vite.config.ts` | API, WS, and runtime images proxy to Go; admin HTML is not proxied | Development admin shell is a visual preview; verify the real gate on Go |
| `embed.go`, `Makefile` | Frontend-first build embeds `web/dist`; committed `.gitkeep` allows clean-checkout compilation | Bundle new assets through the existing pipeline |
| `openspec/specs/` | No existing capabilities | Both capabilities are new, although the route replacements are breaking changes |
| `.gitignore` | `openspec/` is ignored | Planning files exist locally; delivery must not imply they are Git-tracked |

Legacy frontend tests include source assertions against player/projector files. Keep these files and their pure helpers unmounted in this change so engine evaluation in Change 5 remains possible; do not delete their tests to make the new surface pass.

## Goals / Non-Goals

**Goals:**

- Establish reusable presentation contracts that do not import game wire types or business state.
- Make every advertised destination navigable in dev and the embedded build, with explicit misses and practical browser acceptance checks.
- Make the visual choices concrete enough to implement: token values, fonts, layout behavior, component states, and a finite route table.

**Non-Goals:**

- No new backend domain, database migration, account authorization system, or commerce API.
- No new routing/state library, client-side navigation framework, Storybook deployment, dark theme, or broad legacy cleanup.
- No commitment to a commerce currency or money representation. Preview price/time slots use unavailable text, not game formatting helpers or active countdowns.

## Decisions

### 1. Replace the mounted website surface and retain the reusable backend

`App.tsx` will mount new storefront/account/admin layouts and preview pages. It will stop importing `PlayerView`, the old `AdminView`, and `ScreenView`. There will be no `/legacy`, `/game`, or alternate `/screen` alias. Keep the dormant game source, engine, hub, database, and tests available for later evaluation, without a route into them.

New components receive small presentation props, not `PlayerView`/`AdminView` snapshots. They do not initialize game sockets, restore player tokens, issue joins, or call game mutation endpoints. The existing backend may still perform its normal game boot/recovery work; this change is retirement of the game UI, not removal of the game server.

Alternative considered: retain game routes beside the new website. Rejected because it adds navigation and theme compatibility work the roadmap explicitly permits us to avoid. Bulk deletion is also deferred because it would expand the migration and discard code intended for later evaluation.

### 2. Keep explicit route resolution and native document navigation

Evolve `web/src/lib/route.ts` into a pure resolver that returns a page identifier, layout identifier, and optional slug. Use real anchors for navigation; do not intercept clicks or add `pushState` routing. Each navigation loads the same built SPA at its destination. This preserves new-tab, Back/Forward, refresh, and the server admin gate without a new dependency or partially implemented history layer.

Normalize a single trailing slash except for `/`; match path segments exactly and case-sensitively. Query and fragment do not change page identity. Dynamic detail paths accept exactly one nonempty slug segment and have no existence lookup yet. Do not interpolate raw route text as HTML or embed admin keys into links. Use page-specific document titles, one `h1`, an `aria-current="page"` marker on the exact current navigation destination, and a skip link to `main`.

| Paths | Layout | Foundation contents |
|---|---|---|
| `/` | Storefront | Brand introduction, preview notice, illustrative lot cards, links to auction/lot indexes |
| `/auctions` | Storefront | Auction-index preview with an example detail link |
| `/auctions/:slug` | Storefront | Generic auction-detail preview, no persisted event or schedule claim |
| `/lots` | Storefront | Small illustrative card grid and static filter/field examples, no search behavior |
| `/lots/:slug` | Storefront | Generic lot-detail composition: image, metadata slots, unavailable price/time, disabled bid with explanation |
| `/account` | Account | Public account-feature introduction; no identity or profile data |
| `/account/watchlist`, `/account/bids`, `/account/orders` | Account | Distinct titled unavailable-feature previews |
| `/admin` | Admin | Foundation overview after the existing gate |
| `/admin/auctions`, `/admin/lots`, `/admin/orders` | Admin | Distinct titled content-management previews; no CRUD or order updates |
| `/screen`, `/screen/*`, all other paths | Storefront not-found | 404 heading, explanatory copy, home link; `/admin/*` misses still pass the Go gate first |

All arbitrary matched slugs display a generic preview rather than fabricated records; record availability becomes Change 2/3 behavior. Login/register, order-detail, checkout, and analytics routes are not advertised or implemented yet. The 404 is a frontend view; document responses remain HTTP 200 through the SPA fallback. Reserved asset misses use real HTTP 404.

Alternative considered: add a client routing library now. Native document links meet the present contract; a later change can introduce client navigation while preserving URLs. Full navigation resets transient preview state, which is acceptable because there is no active business form or live bidding state.

### 3. Confirm the light-theme token system

Keep AuctionLab as the product name and use **Collected Stories** as the visual concept/tagline. Centralize semantic tokens in `web/src/index.css`, mapped into the existing styling layer. Scope website theme changes with a website root class where needed to avoid unintentionally restyling retained, unmounted game source. Remove game-specific visual recipes from the shared primitives used by the new website; dormant source need not retain its former appearance.

| Role | Value | Usage |
|---|---|---|
| Background | `#F6F1E8` | Ivory page canvas |
| Surface | `#FFFFFF` | Forms and selected panels |
| Foreground | `#24211E` | Main text and headings |
| Muted foreground | `#625A52` | Secondary readable text |
| Primary / on-primary | `#722F37` / `#F6F1E8` | Main actions |
| Primary hover | `#5B252C` | Main-action hover/pressed background |
| Olive | `#62664D` | Supporting accent and positive state text |
| Brass | `#A38355` | Decorative rules/illustration accents only |
| Error | `#8D2E35` | Invalid text, border, and alert icon |
| Warning | `#715116` | Warning text |
| Control border | `#8A7C6C` | Input/outlined-control boundary |
| Divider | `#D8CDBD` | Decorative separators, never the only control boundary |
| Focus | `#722F37` | 2px outer ring with 2px background-colored gap |

Contrast calculated during planning from sRGB relative luminance (ratios are for these flat pairs, not a rendered accessibility audit):

| Pair | Ratio |
|---|---|
| Foreground / ivory | 14.23:1 |
| Muted foreground / ivory | 6.01:1 |
| Ivory / primary | 8.57:1 |
| Ivory / primary hover | 10.69:1 |
| Olive / ivory | 5.29:1 |
| Error / ivory | 7.25:1 |
| Warning / ivory | 6.46:1 |
| Control border / ivory | 3.60:1 |
| Control border / white | 4.05:1 |
| Brass / ivory | 3.15:1 — unsuitable for normal-sized text |

Use labels plus color for states. Keep badges on ivory/white with the specified dark text; do not introduce unmeasured tinted backgrounds or text opacity for enabled states. Disabled controls use readable muted copy, a muted surface, and an explanation; keep their lack of action visible without relying on low opacity.

| Scale | Choice |
|---|---|
| Heading family | Bundled Noto Serif, 400/600; `Georgia, serif` fallback |
| Body family | Bundled Noto Sans, 400/600; `system-ui, sans-serif` fallback |
| Type sizes | 14px caption, 16px body/form, 20px subheading, 28px section, fluid 36–56px hero |
| Leading | 1.5 body; 1.15 headings; allow long titles to wrap |
| Spacing | 4, 8, 12, 16, 24, 32, 48, 64px |
| Borders / radii | 1px boundaries; 0px editorial cards, 4px controls; no hard offset shadows |
| Content width | 1200px maximum; 16px mobile, 24px tablet, 32px desktop gutters |
| Breakpoints | Below 640px mobile; 640–1023px tablet; 1024px and above desktop |
| Card grid | 1 / 2 / 3 columns at the three breakpoints |
| Targets / motion | Minimum 44px interaction size; up to 150ms color transitions; none for reduced motion |

Alternative considered: preserve the old tokens and add vintage styling per page. Rejected because it leaves conflicting controls and duplicated rules. The original roadmap palette is retained with explicit accessible supporting colors.

### 4. Compose three layouts from a small shared component set

Proposed organization:

- `web/src/layouts/`: `StorefrontLayout`, `AccountLayout`, `AdminLayout`.
- `web/src/components/site/`: site header/navigation, footer, contextual navigation, lot card, status badge, reusable field wrapper, preview notice, and empty/unavailable state.
- `web/src/routes/storefront/`, `routes/account/`, and a distinct admin-preview module: page composition and route-specific headings.
- `web/src/fixtures/`: a few illustrative presentation records only, deliberately separate from WS protocol types and future API clients.

Adapt existing `components/ui/button*`, `input`, `label`, and `card` instead of generating a second primitive library. Preserve semantic button/link behavior and explicit label/error associations. The lot card uses one primary detail link; avoid nested interactive regions. Status examples cover upcoming, live, ended, sold, and unavailable but are visibly illustrations, never live status reports.

Storefront navigation uses compact wrapping links on mobile, avoiding a menu overlay and focus trap. Account/admin section navigation becomes wrapping horizontal links below 1024px and a sidebar at desktop widths. Footer links stay within the route table; do not invent policy/contact destinations. Admin access can remain an operator URL; it need not appear in public primary navigation.

A reusable preview notice is present on every foundation page with plain product copy such as “Website preview — bidding and purchases are not available yet.” Do not expose roadmap change numbers or implementation details in product copy. Form examples use nonsensitive sample text and do not submit; inactive business controls have explanations. A few locally authored vintage illustrations demonstrate camera, clock, or vinyl card proportions; the 15–20 lot seed belongs to Change 2.

Alternative considered: a standalone component-showcase application. Rejected because the page previews already exercise all required shared components without creating another shipped surface.

### 5. Bundle fonts and assets; preserve language and hosting semantics

Vendor WOFF2 files for Noto Serif and Noto Sans, with the English/Vietnamese coverage and required weights above. Record exact font release/source, filenames, license, and attribution when acquiring the files; include the upstream license text. Use local font declarations with swap behavior and verify Vietnamese diacritics in the actual files. Put imported fonts/illustrations under `web/src/assets/` so production bundling fingerprints them. Put any non-imported shared fallback under `web/public/site-assets/`, not `public/assets/` and not the runtime `/images/` mount. Record authored illustrations as project-created assets. Do not use remotely hosted imagery.

Extend `internal/api/static.go` to exclude `/site-assets/*` from SPA fallback alongside `/assets/*`; retain the current HTML and immutable-asset policies. Confirm the development server returns 404 for missing paths in both namespaces, adding a narrowly scoped pre-fallback handler in `web/vite.config.ts` only if required. Do not change `/api`, `/ws`, or runtime `/images` ownership. Update static-server comments/examples to describe website URLs without duplicating the frontend route table in Go.

Reuse the language provider and stored `auctionlab.lang` preference. Keep new website copy in a separate typed dictionary integrated with the provider so game copy and its fixed-count tests need not be rewritten. Shared shell labels, previews, status copy, document titles, and image fallback text must work in both languages. Set document language consistently on initial load and after switching. This preserves observed behavior without adding localization infrastructure.

Alternative considered: system fonts only. They remain fallbacks, but locally bundled fonts provide the consistent serif/sans pairing the roadmap asks Change 1 to confirm. CDN fonts conflict with the offline demo constraint.

### 6. Preserve the admin gate with a documented development boundary

Retain `AdminGate` key checks, HttpOnly cookie behavior, fixed `/admin` redirect, and no-key/wrong-key behavior. Restyle only its inline HTML/CSS to the new palette, thin boundaries, readable form controls, and focus state. It remains a self-contained bilingual entry page using system font fallbacks; shared SPA components use the bundled fonts. Do not add a login API or return-to URL handling in this change.

Vite currently serves `/admin` without the Go document gate. This remains a local development preview containing no data or mutation controls; it is not evidence of authentication. Native links into `/admin` on the built Go origin must request the gated document. All authorization checks and protected deep-link acceptance are exercised through the Go server. Change 4 will establish account/session authorization and its own development contract.

Alternative considered: remove the key gate now or build replacement account auth now. Both would cross Change 1's boundary. There is no need to proxy admin documents to a stale built bundle merely to style empty preview pages during development.

### 7. Validate behavior at the boundaries that changed

- Extend `web/src/lib/route.test.ts` with every route class, query/fragment and trailing-slash cases, exact-prefix misses, extra segments, and retired screen URLs.
- Extend `internal/api/static_test.go` for public/account nested URLs, local fonts/images, `/site-assets/` misses, existing cache policies, and missing hashed assets. Preserve admin-gate behavior tests, adjusting only presentation expectations where necessary.
- Run existing frontend tests, lint/typecheck/build, and Go tests. Preserve game correctness tests; update obsolete route expectations because the route behavior intentionally changes.
- Run `make build` and inspect the built Go application using a temporary database. Visit every route class directly, refresh, and exercise links plus Back/Forward. Check admin URLs before/after a valid capability; do not record the key in screenshots or verification notes.
- Inspect browser layouts at 320, 375, 768, and 1280 CSS pixels, 200 percent text zoom, keyboard-only navigation, both languages, missing image/font fallback, and reduced motion. Check actual component contrast and computed fonts, not only token values.
- With the local server reachable and external origins blocked, cold-load the built website and verify its required assets load. Confirm the new shells open no game sockets or business requests, including with an existing player token in local storage.
- Save concise results and representative screenshots under this change's `verification/` directory during implementation. Label automated checks and manual observations separately; this planning turn supplies no implementation evidence.

Alternative considered: add a new end-to-end testing framework and broad component snapshots. Existing unit/HTTP tests plus targeted browser acceptance are proportionate to this presentation foundation. Add a harness only if an actual regression cannot be covered with those checks.

## Risks / Trade-offs

- [Old bookmarks now reach previews or 404] → Deliberate breaking transition, documented in the proposal and route table; no automatic game-data conversion.
- [Preview content mistaken for a working auction site] → Shared visible notice, unavailable values, disabled/omitted business actions, isolated fixtures, and no game-network connection.
- [Token changes leave old hardcoded primitive styles] → Update primitive recipes as part of the same change and inspect all three layouts plus the Go gate.
- [A router later bypasses document-level admin gating] → Native document links preserve the gate now; account authorization must be enforced server-side on data/actions when Change 4 introduces them.
- [Vietnamese font subset or asset license incomplete] → Verify acquired files and record licenses before accepting the build; block completion rather than silently switching to external fonts.
- [Frontend 404 returns HTTP 200] → Accept for this SPA foundation; asset/API misses retain real error status. Server-rendered SEO/status handling is outside scope.
- [Dormant game code and backend remain] → Keep it unmounted and covered by existing tests until later changes decide reuse/removal; do not advertise backend retirement as completed.
- [Planning artifacts are ignored by Git] → Report their local location accurately; do not change ignore rules or stage files as a side effect of proposing.

## Migration Plan

1. Implement tokens/assets and reusable components, then switch `App.tsx` to the explicit route table and new shells in one change.
2. Restyle the Go gate and handle the public-asset namespace; preserve the frontend-first embed pipeline and `.gitkeep` restoration.
3. Validate with a separate temporary database and browser profile before replacing the demo binary. No existing database or stored player token is deleted or migrated.
4. Update `web/README.md` with the foundation routes, preview status, removed game routes, offline assets, and the dev/admin gate distinction.
5. Roll back by restoring the previous application source/build and binary; no schema downgrade is needed. The preserved database remains usable by the old game version.

No blocking design questions remain. Currency, account semantics, lot persistence, and live bidding contracts are intentionally decisions for their respective later roadmap changes.

# AuctionLab — Collected Stories

A vintage auction **website foundation**, using React, Vite, and TypeScript. All displayed objects and statuses are illustrative previews. Catalog persistence, authentication, search/filtering, watchlists, live bids, checkout, and orders are not implemented. The shared preview notice appears on every page. Disabled controls have adjacent explanations and perform no requests.

## Run and build

From the repository root:

| Command | Behavior |
|---|---|
| `make dev` | Go :8080 and Vite :5173; API, WS, runtime images proxy to Go |
| `make dev-web` | Vite only; sufficient for public, account, and admin visual previews |
| `make test` | Existing Go tests, frontend unit tests, oxlint |
| `make build` | Typecheck + Vite build **before** `CGO_ENABLED=0 go build -o bin/auctionlab ./cmd/server` |

The built binary embeds `web/dist`; Node is not needed at runtime. `dist/.gitkeep` stays committed and is restored by the Vite closeBundle hook after every build. Fonts and illustrations are local; running offline needs only the application server, with no CDN/service dependency. A first dependency install/build may require network access.

## Routes and document navigation

| Path | Layout | Contents |
|---|---|---|
| `/` | Storefront | Introduction and illustrative collection |
| `/auctions` | Storefront | Auction-index preview |
| `/auctions/:slug` | Storefront | Generic auction detail, no stored record lookup |
| `/lots` | Storefront | Illustrative cards and non-submitting field examples |
| `/lots/:slug` | Storefront | Generic lot detail, unavailable price/schedule and disabled bid |
| `/account` | Account | Public introduction with no identity/profile |
| `/account/watchlist` | Account | Saving unavailable |
| `/account/bids` | Account | Bid history unavailable |
| `/account/orders` | Account | Orders unavailable |
| `/admin` | Admin | Management overview |
| `/admin/auctions` | Admin | Publishing unavailable |
| `/admin/lots` | Admin | Creating/editing unavailable |
| `/admin/orders` | Admin | Processing unavailable |
| All other paths, including `/screen` and descendants | Storefront | Explicit 404 with a home recovery link |

`src/lib/route.ts` matches case-sensitive segments, ignoring query/fragment and allowing one trailing slash. Detail routes accept exactly one nonempty slug and always show a generic preview. Slugs are not inserted into HTML. No game alias is provided. The old player/admin/projector source and tests remain unmounted for future evaluation; the backend, database, protocol, and existing game data are preserved. Saved player tokens are not read or restored by the website.

Navigation uses normal `<a href>` document links, with no click interception or History API router. Refresh, direct load, Back/Forward, and new-tab links retain normal browser semantics and the Go document gate. Full navigation resets transient sample input values. Exact active destinations use `aria-current="page"`; each layout has a skip link to `main#main` and a shared footer. Context navigation wraps on mobile and becomes a sidebar at 1024px.

The frontend 404 is a view: Go document fallback returns HTTP 200 with `Cache-Control: no-cache`. Missing `/assets/*` and `/site-assets/*` files return real HTTP 404 without SPA HTML in Vite and Go. Only hashed `/assets/*` files are immutable; hand-managed files belong in `public/site-assets/`. `/api`, `/ws`, and runtime `/images` remain owned by the backend.

## Admin and account boundary

Vite serves a public, empty **visual admin preview**, with no private data or mutations. It does not implement or prove authorization. The Go origin protects `/admin` and its subtree with the existing admin capability gate. Read the entry link from the server log. A correct key sets the existing HttpOnly, SameSite=Lax, 12-hour capability cookie and redirects to literal `/admin`, removing the key from the URL. Nested navigation/refresh then uses the cookie. Missing capability gets the bilingual gate; a wrong nonempty key returns 401 without echoing it. The gate uses self-contained CSS and system fallbacks. No sign-in API, return-to mechanism, or new account authentication exists. Account pages are public feature previews, not simulated signed-in sessions.

## Presentation components

Tokens live in `src/index.css`: ivory `#F6F1E8`, charcoal `#24211E`, burgundy `#722F37`, olive status accents, decorative brass. Headings use local Noto Serif 400/600 and Georgia fallback; body uses Noto Sans 400/600 and system-ui fallback. Type roles are 14/16/20/28px with 36–56px fluid hero, 1.5 body / 1.15 heading leading. Prices/times use `.numeric` tabular numerals. Spacing is 4/8/12/16/24/32/48/64px. Controls use 1px borders, 4px radius, 44px minimum targets, visible focus with an ivory gap, and readable disabled/error states. No hard offset shadows. Reduced motion disables nonessential transitions. Grid: 1 column below 640px, 2 at 640px, 3 at 1024px; content max 1200px with 16/24/32px gutters.

- `components/ui/`: existing `Button` (default/outline/secondary/ghost/destructive/link; `asChild` for a real link), `Input`, `Label`, and `Card` family use the shared tokens. Use `type="button"` for UI actions. Never use an anchor as a disabled button.
- `components/site/Presentation.tsx`: `LotCard({title, metadata, status, image?, href})` has one primary detail link; `StatusBadge({status})` accepts upcoming/live/ended/sold/unavailable and always includes text; `LotImage({src?, alt})` handles failures with a local fallback plus translated caption; `PreviewNotice`, `PageHeading({title, intro?})`, `UnavailableState({title, children})` are shared presentation components.
- `Field({id, label, help, error?, ...inputProps})` associates its label, help, and optional invalid/error text. Example: `<Field id="sample" label="Collection" help="Preview only" error="Example error" defaultValue="" />`. Sample fields have no submitting form, no credential inputs, and no business handler.
- `fixtures/site.ts`: three illustrative objects, independent of game wire types and API clients. A long title exercises wrapping. Arbitrary detail slugs do not assert record existence.
- `lib/site-copy.ts`: typed EN/VI website dictionary and `useSiteCopy()` integrate with the existing language provider and `auctionlab.lang` preference. Use `t('imageAlt', {object: title})` for text interpolation; React escapes output. Game copy and its fixed-count tests remain unchanged. App updates document title/language for each route and language.
- `layouts/SiteLayout.tsx`: shared header/footer plus storefront, account, admin shells, all composed with native links.

Asset sources, exact package releases, font metadata/licenses, checksums, and illustration provenance are in `src/assets/README.md`. Fonts/illustrations imported from `src/assets` are fingerprinted (small SVGs may be embedded as local data URLs); the shared fallback is `/site-assets/image-unavailable.svg`. No remote images or styles.

## Verification

Route and bilingual-copy tests cover matching, misses, completeness, and interpolation. Static HTTP tests cover deep links, fonts/images, reserved asset misses and cache policy; admin tests retain key/cookie/redirect and bilingual error semantics. Existing game tests remain in place. Change-local browser observations, screenshots, computed contrast, and acceptance mapping are under `openspec/changes/archive/2026-10-19-establish-vintage-design-and-site-foundation/verification/` (this completed archive and its main specs are explicitly tracked; the general OpenSpec ignore rule remains in place).

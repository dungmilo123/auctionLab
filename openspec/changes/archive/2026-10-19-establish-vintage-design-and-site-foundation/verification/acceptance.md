# Foundation acceptance

Change: `establish-vintage-design-and-site-foundation` (spec-driven). Verified 2026-10-19T07:39:04.208Z.

Result: **PASS** for the website foundation described by both delta specs. This is not completion of catalog, account authentication, bidding, payment, checkout, or order functionality. No database migration/deletion was implemented. The browser run uses a new temporary `/tmp/auctionlab-foundation-*` database, randomly generated in-memory capability, and a temporary local server. Existing user data is untouched. No capability, cookie value or credential-bearing URL is saved in this record, browser JSON or screenshots.

## Automated checks

| Check | Result |
|---|---|
| `make test` | Pass: all Go packages; 18 frontend files / 472 tests; oxlint |
| `make build` | Pass: TS project typecheck + Vite before `CGO_ENABLED=0 go build` |
| Embedded artifact | `bin/auctionlab`, statically linked ELF; `web/dist/.gitkeep` present |
| `openspec validate establish-vintage-design-and-site-foundation --strict --json` | Pass, no issues |
| `git diff --check` | Pass |
| Browser route loads + refresh | 38 cases (19 Vite + 19 Go) |
| Responsive/text/reduced-motion matrix | 88 cases |
| Keyboard focus traversal | 150 website steps plus gate/skip-link interactions |
| Computed contrast | 14 website + 9 gate text pairs pass; control boundaries/focus checked |
| Cold load / external origins blocked | Pass, local font/illustration rendering and language persistence |
| Forced font/image failures | Pass, readable system font and local/caption fallbacks |
| Saved game identity | Exact `auctionlab.player_token` ignored; zero game socket or business activity |

Environment: Linux amd64; Go 1.26.4; Node 24.17.0; Playwright 1.64.1; Chromium 156.0.8078.4; temporary Fontkit 2.0.4 for WOFF2 glyph verification. Playwright/Fontkit are installed under `/tmp/auctionlab-browser`, not in the project dependencies or lockfile. The connected UI tool had no available browser, so checks ran in local headless Chromium.

A pre-existing retained-game test expected `lobby.budgetCaption` in PlayerView but that caption was absent on the clean starting tree. Restored the single caption line in the unmounted component. The test and all other game source/tests were retained. No test was deleted or weakened.

## Requirement reconciliation

| Capability | Requirement | Evidence |
|---|---|---|
| vintage-design-system | Shared vintage visual language | Tokens/primitives; storefront/account/admin screenshots; computed serif/sans families, no old orange/hard shadows. |
| vintage-design-system | Readable text and identifiable states | visual-checks.md calculated pairs, 14 website + 9 gate rendered text measurements; textual status/error examples; default/hover/focus checks. |
| vintage-design-system | Accessible shared controls | 150 Tab steps; native links and skip-link Enter; label/help/error IDs; 44px target matrix; disabled controls and Enter in sample field generate no request. |
| vintage-design-system | Responsive and motion-aware layouts | 88 matrix cases: 320/375/768/1280px, both languages, 100/200% text size, reduced motion. No overflow, clipped elements or undersized controls. |
| vintage-design-system | Reusable lot presentation | Three isolated illustrations; long vinyl title; one detail link per card; unavailable pricing; local fallback and total image-failure screenshots. |
| vintage-design-system | Locally available fonts and illustrations | External origins blocked in cold context; all eight local font faces load; CDP confirms actual fonts and system fallback; Fontkit glyph coverage; src/assets/README.md provenance and unmodified licenses. |
| vintage-design-system | Consistent language presentation | site-copy tests (completeness/interpolation), retained i18n tests; EN/VI matrix and real link navigation; language/document title update; persisted auctionlab.lang. |
| website-navigation | Explicit website destinations | 40 route unit cases; 19 route browser cases per origin, including arbitrary slugs, account/admin sections, unknown and retired paths. |
| website-navigation | Navigable layout shells | Exact active destinations, native header/footer/context links; direct link destinations inspected; Back/Forward/new-tab, skip main, narrow navigation and sidebar screenshots. |
| website-navigation | Direct URL loading | 38 browser direct-load + refresh cases across Vite/Go; Go document no-cache; authorized admin nested load/refresh. |
| website-navigation | Preserve the admin capability boundary | No-key gate 200 and wrong-key 401 on /admin and /admin/lots; no SPA/key echo; correct query 303 to /admin; HttpOnly SameSite=Lax cookie; gate keyboard/error screenshot; original authorization tests retained. |
| website-navigation | Honest preview boundaries | Notice on all website pages; no submitting forms, unavailable prices/schedule and disabled business actions; no production external, game WebSocket, /api, /ws, /images or non-GET business request. Exact saved auctionlab.player_token is ignored on storefront/account. |
| website-navigation | Retire the old game route surface | App imports only new shells/pages; / and /admin are previews, /screen and descendants are storefront 404; no legacy links; old game source/tests/backend/data retained. |
| website-navigation | Recoverable not-found view | 404 headings and home link in shared storefront; unknown/admin-unknown after gate; recovery link actually clicked; document remains HTTP 200/no-cache as documented. |
| website-navigation | Asset failures remain asset failures | Go static unit tests and real HTTP; Vite request checks with Accept:text/html; missing font/SVG under both namespaces returns real 404 without SPA/immutable header; real font and fallback load; hashed assets immutable; unknown API JSON 404. |

## Browser observations

- Go gate: no-key 200 and wrong-key 401 on /admin and /admin/lots; no SPA leakage/key echo; correct key 303 to literal /admin, HttpOnly SameSite=Lax cookie, key removed; keyboard and nested refresh pass.
- Go direct load/refresh: 19 route cases; exact layouts/headings, native links, active states, Back/Forward/new-tab, skip-link focus and label/help/error wiring pass.
- Responsive matrix: 80 website cases + 8 bilingual gate cases; 320/375/768/1280px, 100/200% text size, both website languages, reduced motion. No horizontal overflow or sub-44px targets.
- Rendered contrast: all measured text pairs >=4.5:1; input/invalid/outlined boundaries >=3:1; primary default/hover and focus match the checked palette. Disabled copy is readable; no text opacity.
- Cold browser with external origins blocked: all 8 local font subsets/weights load with Vietnamese text; local illustrations render; preference survives a real link. Forced font/image/fallback failures retain readable text/captions, card dimensions, and detail links. Saved game tokens cause no restore, game WebSocket, or business request.
- Embedded HTTP: documents no-cache; all built fonts/CSS/JS immutable; fallback SVG local and non-immutable; both namespaces return 404 without HTML or immutable caching; unknown API stays JSON 404.
- Vite: every route direct-loads/refreshes; Back/Forward/new-tab pass; admin is a public empty visual preview; real local fonts/fallback load and missing fonts/images in both reserved namespaces return 404 even with Accept:text/html.

## Visual review and limits

`visual-checks.md` contains calculated and actual rendered contrast, the layout matrix interpretation, and inspected screenshots. Reviewed storefront desktop; account/admin/404 narrow/desktop; bilingual gate; 200% Vietnamese copy; local image fallback and total asset failure. No clipping, page overflow, blank disabled labels, misplaced focus or unreadable copy remains in the recorded matrix. Screenshots are representative, not pixel snapshots used as test oracles.

Text enlargement uses 200% root font size in headless Chromium, with every content/control text role expressed relative to the root. No physical mobile device, other browser engine, actual browser toolbar text-zoom control, or screen-reader session was exercised. This is bounded browser acceptance, not universal accessibility certification.

Vite `/admin` is a public visual shell without business actions; the Go-served document is capability-gated. Account views are public and contain no profile data. Frontend 404 views are intentionally HTTP 200; asset/API failures retain true error status. Native links reload documents and reset transient sample fields. These behaviors match `web/README.md` and both specs. No new commerce/currency/authentication contract is implied.

## Reproduce

From repository root, run `make test` and `make build`. Start `make dev-web` for the Vite :5173 checks. With Playwright available, run:

```sh
AUCTIONLAB_PLAYWRIGHT=/tmp/auctionlab-browser/node_modules/playwright/index.mjs node openspec/changes/archive/2026-10-19-establish-vintage-design-and-site-foundation/verification/browser-checks.mjs
```

The driver starts/stops its own Go server on :18281 with a new temporary database and blocks external origins for the cold-load case. It writes sanitized paths and observations to `browser-results.json` and representative PNGs to `screenshots/`. It never records the generated capability. Token glyph/asset provenance and file checksums are in `web/src/assets/README.md`.

Remaining failures: **none within the specified foundation acceptance**.

## Archive handoff — 2026-10-19

All 22 tasks are complete. Both capabilities were synced to main specs with Purpose, requirements, and scenarios preserved and passed strict validation. The completed change was moved into the dated archive. At the user’s explicit request to commit and archive, this archive, its existing evidence, and the two main specs are explicitly included in the implementation commit while the general OpenSpec ignore rule is preserved. Browser JSON and screenshots retain the original run evidence. This archive is a website-foundation delivery, with the limitations above unchanged.

# Change 9 verification — 2026-10-23

Implementation and acceptance are complete: **47/47 tracked tasks**. This report
covers `polish-auction-website-experience`. The user subsequently requested commit
and archive; all 11 delta capabilities are synced to main specs and the change is
archived on 2026-10-23. Earlier archived acceptance checklists remain unchanged.

## Result and visual critique

The first featured published collection now anchors home. Shared object cards use
larger local imagery, compact schedules, exact labeled prices and separate save
controls. Lot inspection shares thumbnail selection with a focus-contained enlarged
gallery. The auction column presents condition and all enabled fulfillment terms
before submission. Account, checkout, history and administration share the same
viewing-room palette, icon sizes, forms and state vocabulary. Buyer labels use
ordinary payment/fulfillment language; local simulation and provenance remain.

Compare [baseline home](before/1280-_.png), [first home](first-home.png),
[first lot](first-lot.png) and [first mobile](first-mobile.png). The original camera
placeholder, capability inventories and repeated long card schedules competed
with actual objects. The revised composition gives collection media one focal
role and keeps utility controls quiet. [Design reference](design-reference.md) and
[first critique](first-critique.md) record the hierarchy and follow-up decisions.

The later utility review checked profile, checkout, catalog editors and analytics.
It corrected inline icon alignment in legends, grouped contact/delivery/payment,
shortened account guidance and emphasized key value metrics without changing
formulas. Long histories remain in normal flow; no sticky bid overlay conceals
terms. [All artwork](media-embedded.png) shows distinct object silhouettes, related
details and composed covers; [original artwork](media-before.png) supplies the
comparison. These are authored vectors, not photographs or condition evidence.

Normal before/first-pass comparisons use the initial stored fixture. Later route
and transaction matrices intentionally add real test collections/orders to that
same disposable schema. Counts and scheduled phases therefore evolve; they are
not fabricated replacements or a claim of identical final catalog contents.
The original analytics fixture deliberately marks some stock prose non-demo;
its authored text is preserved by the provenance-gated adapter, as required.

## Commands and focused regression evidence

| Check | Result / evidence |
| --- | --- |
| `make test` with dedicated `TEST_DATABASE_URL` | Passed; [log](make-test.log), 37 frontend files / 560 tests, Go tests and lint |
| `make test-race` with the same dedicated test database | Passed; [log](make-test-race.log) |
| `make build` | Passed, frontend before CGO-free Go embedding; [log](make-build.log) |
| Strict change validation | Passed; [log](openspec-validation.log) |
| Focused PostgreSQL catalog/store/API checks | Passed; [log](focused-go.log) |
| Scoped whitespace/error check | `git diff --check` passed |
| Final health | PostgreSQL ready, supported schema version 6 |

New focused tests cover exact stock-field adaptation, edited/non-demo/unrelated
content, historical owner snapshots, immutable amounts/receipts, one shared list
read, duplicate suppression, unknown membership recovery and late identity-bound
reads/writes. Existing query, selection, money, session, auction-feed, checkout,
privacy, inert-rendering, seed/repeat/collision and publication checks still pass.
No migrations, money rules, payment IDs, authorization rules or auction windows
were changed. Real PostgreSQL was used; no runtime SQLite fallback was introduced.

## Browser evidence

All browser runs use isolated Chromium contexts and synthetic fixture data. See
[setup/methodology](README.md), [fixture source](fixture.go) and [baseline metadata](baseline.json).
The dedicated `auctionlab_polish_test` database and randomly named marked
`auctionlab_demo_change9_*` schema are separate from operational data. Credentials
and capability values live only in temporary mode-0600 input manifests.

| Evidence | Coverage |
| --- | --- |
| [Route matrix](matrix.json), `after/` screenshots | 388 EN/VI observations across storefront, events/lots, account/auth/watchlist/bids/wins, checkout/history/detail, admin overview/editors/orders/analytics, gate and 404; 320/375/1280 CSS px and 84 native zoom cases |
| [State matrix](states.json) | 176 bilingual narrow observations: loading, empty, no-match, long title, failed media, missing records, unavailable services, access denial and invalid selection |
| [Form zoom supplement](zoom-forms.json) | 20 additional native-200%-zoom/reduced-motion cases: checkout field errors, paid receipts, active shipping input and login/registration errors, EN/VI in both builds |
| [Interaction checks](interactions.json) | Keyboard selection/Escape/focus return, rapid language reopening, query/image preservation, one shared watchlist read per validated document, one lot view and zero extra views/writes from language/gallery selection |
| [Purchase journeys](journey.json) | Two bidders in each build; save/login, competing bids, sold result, exact review/confirmation, refresh, foreign-owner 404 and shipping/pickup completion |
| [Privacy checks](privacy.json) | Active receipt feedback, identity change with delayed reads, obsolete private DOM removal, capability-loss cleanup and distinguishable private recommendation removal |
| [Operator recovery](operator-states.json) | 120/200-code-point shipping values, native zoom/reduced motion, EN/VI value preservation, 422 correction, conflict readback and unknown committed shipment recovery without replay |
| [Module recovery](module-recovery.json) | Missing deferred route code shows localized recovery and deliberate reload; no commerce write is replayed |
| [Media checks](media.json) | All 39 existing paths render in Vite and embedded builds; no external fetches or baked-in demo lettering |

Stress-response substitutions are explicitly labeled presentation fixtures in the
state metadata; they are not proof of server state. Real confirmation/shipment
unknown-result tests let the server commit, replace only the response, then read
the durable record. Each final journey issues exactly two confirmation requests
for two distinct orders. Shipping totals are 4,250 cents; pickup totals are 2,500
cents. Refresh preserves the same receipt; no second confirmation is sent.

Native zoom is set and independently read through `chrome.tabs.setZoom(2)` and
`getZoom`; form supplements assert a 1280-pixel browser becomes a 640-CSS-pixel
viewport at DPR 2. This is not CSS scaling or viewport-only substitution. Reduced
motion is actively enabled, including runtime changes while a portal is open;
CSS animation/transform and the shared runtime policy are checked. No route/state
observation had page-level horizontal overflow. Dense analytics tables retain
intentional internal scrolling.

### Reviewable recordings

- Vite: [shipping buyer](recordings/vite-buyer-shipping.webm), [pickup buyer](recordings/vite-buyer-pickup.webm), [operator](recordings/vite-operator.webm).
- Embedded: [shipping buyer](recordings/embedded-buyer-shipping.webm), [pickup buyer](recordings/embedded-buyer-pickup.webm), [operator](recordings/embedded-operator.webm).
- Interaction/motion: [Vite controls](recordings/vite-interactions.webm), [embedded controls](recordings/embedded-interactions.webm), [Vite timing trace](recordings/vite-performance.webm), [embedded timing trace](recordings/embedded-performance.webm).
- Privacy/recovery: [Vite](recordings/vite-privacy.webm), [embedded](recordings/embedded-privacy.webm), [Vite operator failures](recordings/vite-operator-states.webm), [embedded operator failures](recordings/embedded-operator-states.webm).

### Earlier acceptance gaps

Current evidence covers the affected surfaces from Change 4 tasks 7.3/8.3, Change 5
8.4, Change 6 7.3, and Change 7 7.5/7.6: accounts/watchlist, gallery/bids,
checkout/errors/receipt, history/admin fulfillment and privacy restoration. It
adds native zoom, active/runtime reduced motion, bilingual narrow layouts,
maximum shipping values and delayed/unknown-state recordings. Their archived
checkboxes and historical limitations were neither edited nor retroactively
marked complete; the new evidence belongs to Change 9.

## Motion, delivery cost and measured limits

[Performance results](performance.json), [baseline build](baseline-build.json),
[final build](final-build.json) and [fixed-path media transfer comparison](media-transfer-comparison.json)
record sizes, request paths, timings and settings without bodies or credentials.

| Distribution metric | Baseline | Final |
| --- | ---: | ---: |
| Production public JS entry | 441,979 bytes | 552,692 bytes |
| Public entry gzip (local measurement) | 133,668 bytes | 179,357 bytes |
| All distributed JS, including deferred routes | 441,979 bytes | 634,974 bytes |
| All distributed JS gzip | 133,668 bytes | 204,367 bytes |
| All 39 catalog SVGs | 29,408 bytes | 183,042 bytes |

Account/admin modules are deferred; the public route does not transfer them.
Motion uses the smaller `m`/`domAnimation` feature set. The final public entry
remains above Vite's 500 KB warning threshold; this is an explicit delivery-cost
tradeoff, not a hidden warning or a claim of smaller total code. Fonts remain
local and unchanged. Upgraded media deliberately costs more than the original
geometric placeholders: fixed HTTP paths compare event-1 482→4,857 bytes,
lot-04 795→2,877, and event-2 482→14,375, all HTTP 200 without external requests.

The single coordinated home sequence has three elements (opacity/transform tracks)
and completes within 650 ms; panels use 240 ms and confirmed feedback 150 ms.
Loading reserves the hero image region. No decorative animation remains running
at rest. Final observed home layout shift was approximately 0.00004 in Vite and
0.00123 embedded. These are local observations, not a production performance SLA.

The matching 375x900 local gallery trace uses 150 ms latency, 200,000 bytes/sec
download, 100,000 upload and 4x CPU slowdown. Baseline click-plus-screenshot took
68 ms; final embedded took 71 ms (38 ms for the click itself), Vite 109 ms (74 ms
click). The baseline network capture has a warm JS/font cache; encoded/file bytes,
not its zero cached transferSize, are used for size comparison. Vite's unbundled
development module traffic is recorded but is not compared as production bundle
transfer. Fixture evolution changes the home image set, so fixed-path media
comparisons supply the like-for-like byte measurement.

Primary media is eager, secondary images are lazy with reserved dimensions.
Media/font/icon/route-module delivery stays on the application origin. Recorded
journeys and timing checks show zero external requests and zero legacy `/ws`
game sockets. Vite's own HMR socket is separately excluded from that game count.
Language, selection and motion do not trigger a commerce mutation or another
lot-view record. Private surfaces have no retained animated exit; public menu
exit content becomes inert while leaving, and obsolete owner data is removed
independently of animation completion.

## Spec/task coverage and handoff

| Capability / task groups | Evidence |
| --- | --- |
| vintage-design-system; groups 1–3 | Design reference, before/first/final images, all-media comparison, tokens, bilingual states and icon/field inspections |
| vintage-catalog; group 3 | Bounded adapter tests, PostgreSQL seed/publication checks, 39-path media manifest, raw-record/receipt preservation |
| website-navigation; group 4 | Native GET/history/new-tab checks, language/focus/rapid reversal, gate/cache/cookie regressions, module recovery |
| auction-discovery; group 5 | Existing selection/query tests, real collection hero, gallery/terms hierarchy, state/zoom matrices and bidding recordings |
| bidder-accounts; group 6 | Shared owner-list tests, safe return/profile/watchlist checks, immediate identity cleanup and two independent bidders |
| winner-checkout and demo-payments; group 7 | Grouped form/review, exact totals, unknown readback, durable receipts, foreign-owner denial and lifecycle tests |
| order-history and demo-fulfillment; groups 7–8 | Committed timelines, inert shipment strings, one-step progression, maximum values and conflict/unknown recovery |
| lot-recommendations; groups 3/5/9 | Natural reasons, independent recovery, provenance-scoped copy and obsolete private-card cleanup |
| website-motion; group 9 | Shared timing/runtime policy, recorded entrance/panels/feedback, idle-loop and private-lifecycle checks |
| integrated acceptance; group 10 | Real PostgreSQL test/race/build, both build modes, matrices, recordings, size comparison and strict validation |

No specified acceptance check remains unresolved. The size increase and synthetic,
local nature of the performance measurements are the material limits above.
No gateway, carrier lookup, trained ML, currency conversion or database migration
was added. Normal payment/fulfillment remains simulation with unchanged wire IDs.

The unrelated `skills-lock.json` and concurrent `.gitignore` change are preserved
outside authored scope. `AGENTS.md`, archived evidence and migrations 1–5 are
unchanged. Active README/frontend guidance and ROADMAP describe this presentation
and the retained simulation boundary. See [the archive record](../ARCHIVE.md)
for lifecycle completion and spec validation limits.

Normal main-spec validation passes for all 20 capabilities. Strict main-spec
validation reports long-description warnings in existing and synced requirements
(8 pass, 12 fail under strict warning handling); no semantic validation errors were
reported. The approved MODIFIED requirement text is preserved. Strict change
validation passes. Logs: [normal specs](main-spec-validation.log),
[strict specs](main-spec-strict-validation.log),
[change after sync](archive-change-validation.log).

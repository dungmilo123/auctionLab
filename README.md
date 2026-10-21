# AuctionLab

Vintage auction catalog and discovery website with a retained classroom-game backend. Go serves an embedded React frontend; PostgreSQL 18 is a required separate service. Published events and lots support public search, category/planned-phase filters, deterministic sorting, galleries and fulfillment disclosures. Commerce accounts support durable sessions, self-profiles and private watchlists. Catalog publication stores immutable configuration; independent timed auctions support manual bids and durable results. Private winner checkout with local simulated payment is available; owner order history and capability-authorized simulated fulfillment are available.

See [PostgreSQL setup and operations](docs/postgresql.md), [catalog/frontend contracts](web/README.md), and [the roadmap](ROADMAP.md). Run `make build` for the frontend-first no-CGO application binary. Database-backed tests require an explicit isolated `TEST_DATABASE_URL`.

Implementation and acceptance evidence: [archived change report](openspec/changes/archive/2026-10-19-manage-vintage-lots-and-auction-events/verification/REPORT.md).

Public discovery routes: `/`, `/auctions`, `/auctions/:slug`, `/lots`, `/lots/:slug`. Seeded drafts become public only after explicit review/publication. Planned windows describe saved schedules; the separate lot-detail execution panel shows live bidding and confirmed results. Anonymous discovery never restores retained game sessions. See the [discovery acceptance report](openspec/changes/archive/2026-10-19-build-auction-discovery-experience/verification/REPORT.md).

Commerce accounts use independent PostgreSQL sessions and safe self-profiles. Public registration creates bidders; account admin roles do not replace the existing catalog capability gate. See [account API](web/README.md#commerce-account-api) and [identity policy](internal/account/README.md). Session-owned bidding and participation are available; private winner checkout with simulated payment is available; owner history and simulated fulfillment are available. Accounts never reuse game players/tokens or change game revisions.

For explicit demo accounts, stop the server, supply `DEMO_BIDDER_ONE_PASSWORD`, `DEMO_BIDDER_TWO_PASSWORD`, and `DEMO_ADMIN_PASSWORD` privately, then run `./bin/auctionlab --seed-accounts`. It provisions two demo bidders and an admin transactionally, prints status only and exits. Repeats preserve existing credentials and saved lots without password inputs. Normal boot installs no defaults. See [provisioning](docs/postgresql.md#explicit-demo-account-provisioning) for fixed reserved-domain identifiers and flag boundaries.

[Archived account/watchlist verification](openspec/changes/archive/2026-10-20-add-bidder-accounts-and-watchlists/verification/REPORT.md) records passing implementation checks and the two pending exact-200%-zoom acceptance tasks. See the [archive record](openspec/changes/archive/2026-10-20-add-bidder-accounts-and-watchlists/ARCHIVE.md).

### Timed commerce auctions (schema 3)

Normal startup activates every previously published immutable schedule before opening the listener. Future lots wait, currently eligible lots open, and expired lots with no accepted commerce bids close unsold at their original deadline (with the actual recording time retained). A 250 ms worker adopts new publications and reconciles independent lots across overlapping events. Maintenance/import/seed commands do not start it. Public catalog schedule filters and opening-price sorts retain their existing meaning.

For a fresh bid-to-win rehearsal, prepare **new drafts** with a future opening and staggered closes, then publish through catalog management. Seeding only prepares drafts and does not invent bids. Never edit or rebase a previously published event to replay a demo. Use two independently signed-in commerce accounts, explicitly submit increasing bids, and inspect the committed sold/unsold result and `/account/bids`. Watching/saving never bids. Sold wins create one owner order; winners can review and confirm a local simulated payment. Owner history and simulated fulfillment are available. See `docs/postgresql.md` for the stopped-writer schema-6 upgrade and backup-based rollback.

## Winner checkout and simulated payment (Change 6)

Normal commerce closure now commits one `awaiting_payment` order with each sold result. Unsold lots create none. Normal startup also adopts historical sold results missing an order before HTTP readiness, in bounded batches; original closing/result times remain unchanged and the new order creation time records adoption. Migration, import and account/catalog maintenance do not perform adoption or payment. Startup fails if a required snapshot cannot be created; correct the underlying published-data issue through an explicit recovery decision, never fabricate missing terms or silently discard a winner.

Winners open **Unpaid wins** at `/account/wins` or follow their won bid/lot participation to `/account/checkout/<orderId>`. The owner reviews frozen lot/fulfillment terms, contact and shipping address (or pickup without an address). Shipping adds the published flat fee; pickup adds zero. Both `demo-card` and `demo-bank-transfer` are local simulations: no money is charged and no card/bank details are collected. Confirmation creates exactly one durable receipt and removes the order from unpaid wins. Its direct checkout URL remains a read-only paid confirmation after refresh/restart. Owner history, admin order operations and simulated fulfillment tracking are available.

A timeout or lost response is an **unknown outcome**. Read the stored order first; if still unpaid, deliberately retry the same retained intent. Login/reload/reconnect never confirms automatically. Unsent contact/address values remain only in component memory and clear on identity changes/restoration. See `internal/order/README.md` for limits and API/error contracts.

For a new demo run, stop the writer to provision accounts with operator-supplied passwords and seed new draft catalog content, then publish a complete future-scheduled event through the catalog gate. Run two commerce accounts through eligible bidding and wait for closure. Keep terminal results, published schedules and receipts intact; the retained game reset does not reset commerce. Normal startup installs no credentials and never rewinds past schedules. The disposable acceptance preparer under this change's `verification/prepare` uses an explicit fixture clock and refuses operational schemas; it is not an operator reset tool.

## Order history, simulated fulfillment and disposable rehearsal

Buyers open `/account/orders` and `/account/orders/<orderId>` for stored purchase history, immutable receipts/contact and the shared timeline. Operators use capability-authorized `/admin/orders` with status/method filters. Shipping advances paid → preparing → shipped → completed; pickup advances paid → ready_for_pickup → completed. Shipment text is manual and every delivery/payment is simulated. Paid checkout keeps its original receipt at every later stage.

For a fresh complete run, supply the three operator password environment inputs, stop any target writer, choose a **new disposable** schema, and run:

```sh
./bin/auctionlab --prepare-demo --demo-schema auctionlab_demo_rehearsal --seed-at 2026-10-21T00:00:00Z
./bin/auctionlab --demo-status --demo-schema auctionlab_demo_rehearsal
DATABASE_SCHEMA=auctionlab_demo_rehearsal ./bin/auctionlab
```

Review/edit future draft schedules and publish through catalog management, bid with two accounts, wait for closure, pay as the owner and complete shipping/pickup as the operator. To replay, stop the writer and back up valuable rehearsal data, then explicitly clear/reseed only that marked target and inspect the new generation:

```sh
./bin/auctionlab --reset-demo --demo-schema auctionlab_demo_rehearsal --confirm-demo-schema auctionlab_demo_rehearsal --seed-at 2026-10-22T00:00:00Z
./bin/auctionlab --demo-status --demo-schema auctionlab_demo_rehearsal
DATABASE_SCHEMA=auctionlab_demo_rehearsal ./bin/auctionlab
```

The whole marked schema's rehearsal accounts/sessions, catalog, bids/results, orders/payments/fulfillment and retained game data are erased; migrations, categories and capability configuration remain. Other schemas/media remain untouched. Prefix/marker/exact confirmation and exclusive ownership are required; unmarked populated targets are refused. No automatic publication/payment/reset exists. See [the complete write scope and recovery drill](docs/postgresql.md#disposable-full-demo-preparation-status-and-reset) before using reset. Unknown reset outcomes require status inspection before any retry.

Change 7 acceptance: [implementation and evidence report](openspec/changes/archive/2026-10-21-add-order-history-and-demo-fulfillment/verification/REPORT.md) records 39/41 tasks. Exact 200% browser zoom and active reduced-motion acceptance remain open; earlier archive gaps are preserved. Normal main-spec validation passes; strict validation reports length warnings in existing and synced requirement descriptions. The change is archived with both acceptance gaps preserved.

### Analytics and rule-based discovery

Capability-authorized operators can open `/admin/analytics` to compare lifetime
published-event participation, committed results and simulated payments. Filters
are shareable: `?event=all&dataset=demo` (also `all`/`non-demo`). Winning totals
include unpaid wins; simulated paid totals include delivery. Distinct bidder
counts cannot be added across events. Definitions, coverage/provenance and
snapshot time appear on the page; a failed read never becomes successful zeroes.
See [metric/API definitions](internal/analytics/README.md).

Home and lot detail show up to six eligible suggestions from `rules-v1`: category
and authored-era matches, owner-watchlist preferences after session validation,
and deterministic closing-soon fallback. These are rules, not trained ML or a
claim of recommendation effectiveness. Anonymous discovery is independent of
account availability. Lot detail can emit a bounded anonymous page observation;
it is not a unique-person count or complete traffic history.

Demo drill: publish future drafts → view a lot → save → bid with two independent
accounts → win → confirm simulated payment → compare summary/event metrics to
SQL → stop writer → inspect generation → explicitly reset the matching marked
demo schema → inspect fresh generation → publish and replay. Reset erases only
that schema's documented rehearsal state, including observations/coverage, and
invalidates its old sessions. It preserves capability/config, migrations,
categories, external media and other schemas. See [schema-6 operations and reset
scope](docs/postgresql.md). Earlier archived accessibility gaps remain open.

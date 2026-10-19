# AuctionLab

Vintage auction catalog and discovery website with a retained classroom-game backend. Go serves an embedded React frontend; PostgreSQL 18 is a required separate service. Published events and lots support public search, category/planned-phase filters, deterministic sorting, galleries and fulfillment disclosures. Commerce accounts support durable sessions, self-profiles and private watchlists. Catalog publication stores configuration; bidding, checkout and orders remain later roadmap work.

See [PostgreSQL setup and operations](docs/postgresql.md), [catalog/frontend contracts](web/README.md), and [the roadmap](ROADMAP.md). Run `make build` for the frontend-first no-CGO application binary. Database-backed tests require an explicit isolated `TEST_DATABASE_URL`.

Implementation and acceptance evidence: [archived change report](openspec/changes/archive/2026-10-19-manage-vintage-lots-and-auction-events/verification/REPORT.md).

Public discovery routes: `/`, `/auctions`, `/auctions/:slug`, `/lots`, `/lots/:slug`. Seeded drafts become public only after explicit review/publication. Planned windows describe saved schedules; bidding remains disabled. Anonymous discovery never restores retained game sessions. See the [discovery acceptance report](openspec/changes/archive/2026-10-19-build-auction-discovery-experience/verification/REPORT.md).

Commerce accounts use independent PostgreSQL sessions and safe self-profiles. Public registration creates bidders; account admin roles do not replace the existing catalog capability gate. See [account API](web/README.md#commerce-account-api) and [identity policy](internal/account/README.md). Bidding, checkout and orders remain unavailable. Accounts never reuse game players/tokens or change game revisions.

For explicit demo accounts, stop the server, supply `DEMO_BIDDER_ONE_PASSWORD`, `DEMO_BIDDER_TWO_PASSWORD`, and `DEMO_ADMIN_PASSWORD` privately, then run `./bin/auctionlab --seed-accounts`. It provisions two demo bidders and an admin transactionally, prints status only and exits. Repeats preserve existing credentials and saved lots without password inputs. Normal boot installs no defaults. See [provisioning](docs/postgresql.md#explicit-demo-account-provisioning) for fixed reserved-domain identifiers and flag boundaries.

[Archived account/watchlist verification](openspec/changes/archive/2026-10-20-add-bidder-accounts-and-watchlists/verification/REPORT.md) records passing implementation checks and the two pending exact-200%-zoom acceptance tasks. See the [archive record](openspec/changes/archive/2026-10-20-add-bidder-accounts-and-watchlists/ARCHIVE.md).

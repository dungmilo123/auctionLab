# AuctionLab

Vintage auction catalog and discovery website with a retained classroom-game backend. Go serves an embedded React frontend; PostgreSQL 18 is a required separate service. Published events and lots support public search, category/planned-phase filters, deterministic sorting, galleries and fulfillment disclosures. Catalog publication stores configuration; bidding, accounts, checkout and orders remain later roadmap work.

See [PostgreSQL setup and operations](docs/postgresql.md), [catalog/frontend contracts](web/README.md), and [the roadmap](ROADMAP.md). Run `make build` for the frontend-first no-CGO application binary. Database-backed tests require an explicit isolated `TEST_DATABASE_URL`.

Implementation and acceptance evidence: [archived change report](openspec/changes/archive/2026-10-19-manage-vintage-lots-and-auction-events/verification/REPORT.md).

Public discovery routes: `/`, `/auctions`, `/auctions/:slug`, `/lots`, `/lots/:slug`. Seeded drafts become public only after explicit review/publication. Planned windows describe saved schedules; bidding remains disabled. Anonymous discovery never restores retained game sessions. See the [discovery acceptance report](openspec/changes/archive/2026-10-19-build-auction-discovery-experience/verification/REPORT.md).

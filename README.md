# AuctionLab

Vintage auction preparation website with a retained classroom-game backend. Go serves an embedded React frontend; PostgreSQL 18 is a required separate service. Catalog publication stores configuration; commerce bidding, accounts, checkout and orders remain later roadmap work.

See [PostgreSQL setup and operations](docs/postgresql.md), [catalog/frontend contracts](web/README.md), and [the roadmap](ROADMAP.md). Run `make build` for the frontend-first no-CGO application binary. Database-backed tests require an explicit isolated `TEST_DATABASE_URL`.

Implementation and acceptance evidence: [archived change report](openspec/changes/archive/2026-10-19-manage-vintage-lots-and-auction-events/verification/REPORT.md).

# AuctionLab

**An online auction house for vintage collectibles.** Bidders browse curated
auction events, save lots to a watchlist and compete in timed auctions. Winners
check out with a simulated payment and track delivery. Operators publish
auctions, fulfil orders and review sales analytics.

Built with **Go**, **React + TypeScript** and **PostgreSQL 18**, shipped as a
single binary.

![AuctionLab home page](docs/screenshots/01-home.png)

**Jump to:** [What it is](#1-what-is-auctionlab) ·
[Feature tour](#2-feature-tour) · [Tech stack](#3-tech-stack) ·
[Architecture](#4-architecture) · [Quick start](#5-quick-start) ·
[Demo walkthrough](#6-demo-walkthrough-10-minutes) · [Tests](#7-running-tests) ·
[Further docs](#8-project-structure-and-further-docs) ·
[Scope](#9-limitations-and-scope)

> **Important:** AuctionLab needs a running **PostgreSQL 18** database. It will
> not start without one. The [quick start](#5-quick-start) shows the fastest way
> to get one (a single Docker command), and a ready-made demo snapshot fills
> every page with data.

---

## 1. What is AuctionLab?

AuctionLab is a complete e-commerce flow built around **timed English
auctions** (open, ascending bids with a fixed increment):

| Who | What they can do |
|---|---|
| **Visitor** | Browse auction events and lots, search, filter and sort, and see suggested lots |
| **Bidder** (registered account) | Save lots to a watchlist, place bids, see whether they are leading or outbid, pay for won lots and follow their orders |
| **Operator** (admin) | Create and publish auction events and lots, move orders through shipping or pickup, view analytics |

Two things are **simulated** on purpose, because this is a course project:

- **Payment.** "Card" and "bank transfer" are local simulations. No money moves and no card or bank details are collected.
- **Delivery.** Shipping and pickup statuses are set by the operator by hand. There is no real carrier.

Prices are stored in **USD cents** (integers, never floating point). The
interface is available in **English and Vietnamese**.

---

## 2. Feature tour

### 2.1 Browse auctions and lots

Published auction events and their lots are public. Visitors can search by
title, description, brand or era. They can filter by category and auction
phase and sort by closing time or price. Each lot has a gallery, condition
notes, specifications and delivery options.

| Auction events | Lot catalogue |
|---|---|
| ![Auctions list](docs/screenshots/04-auctions.png) | ![Lots list with filters](docs/screenshots/05-lots.png) |

![Auction event page](docs/screenshots/06-event.png)

### 2.2 Accounts and watchlist

Anyone can register as a bidder. Sessions are stored in the database, and only
a hash of each session token is kept. Signed-in bidders can save lots to a
private watchlist.

| Sign in | Watchlist |
|---|---|
| ![Sign-in page](docs/screenshots/09-login.png) | ![Watchlist](docs/screenshots/07-watchlist.png) |

### 2.3 Live timed bidding

Each lot opens and closes on a published schedule. The bidding panel shows the
current price, the next minimum bid, the fixed increment and a countdown. Every
bidder sees whether they are **leading** or have been **outbid**. When the lot
closes, the server records the result once: **sold** to the highest bidder, or
**unsold** if nobody bid.

| You are leading | You have been outbid |
|---|---|
| ![Bidder leading](docs/screenshots/02-lot-bidding-leading.png) | ![Bidder outbid](docs/screenshots/03-lot-bidding-outbid.png) |

| My bids (filter by leading, outbid, won, lost) | A closed lot with its recorded result |
|---|---|
| ![My bids page](docs/screenshots/08-my-bids.png) | ![Sold lot](docs/screenshots/16-lot-sold.png) |

### 2.4 Winner checkout (simulated payment)

When a lot sells, the winner gets an order in **Cart · Unpaid wins**. At
checkout they enter contact details, choose **shipping** (adds the published
flat fee) or **pickup** (free), and pick a simulated payment method. They review
the total, confirm, and receive exactly one receipt.

![Unpaid wins](docs/screenshots/10-unpaid-wins.png)

| 1. Enter details | 2. Review order | 3. Payment confirmed |
|---|---|---|
| ![Checkout form](docs/screenshots/11-checkout-form.png) | ![Checkout review](docs/screenshots/12-checkout-review.png) | ![Payment receipt](docs/screenshots/13-payment-receipt.png) |

### 2.5 Order history and delivery tracking

Buyers can see all their orders and each order's timeline:
paid → preparing → shipped → completed, or paid → ready for pickup → completed.

| Order history | Order detail and timeline |
|---|---|
| ![Order history](docs/screenshots/14-order-history.png) | ![Order detail timeline](docs/screenshots/15-order-detail.png) |

### 2.6 Admin: overview and catalogue management

The admin area is protected by a secret **admin link** (see
[Quick start](#5-quick-start)). Operators create events and lots as **drafts**,
then **publish** them. Publishing freezes the schedule and prices, so a live
auction cannot be changed underneath bidders.

![Admin overview](docs/screenshots/22-admin-home.png)

| Auction events | Lots |
|---|---|
| ![Admin auction events](docs/screenshots/20-admin-catalog.png) | ![Admin lots](docs/screenshots/21-admin-lots.png) |

![Editing a draft auction event](docs/screenshots/27-admin-event-editor.png)

### 2.7 Admin: orders and fulfilment

Operators filter orders by status and delivery method and advance each one to
its next stage. Shipped orders get a carrier and tracking reference.

| Order list | Order detail |
|---|---|
| ![Admin orders](docs/screenshots/23-admin-orders.png) | ![Admin order detail](docs/screenshots/24-admin-order-detail.png) |

### 2.8 Admin: analytics dashboard

Charts and KPI cards cover bids, bidders, lot outcomes (sold or unsold),
winning totals and simulated payments. They can be compared across auction
events. Each metric's definition and its data snapshot time are shown on the
page.

![Analytics dashboard](docs/screenshots/25-admin-analytics.png)

### 2.9 Recommendations (rule-based)

The home page and each lot page suggest up to six lots. The suggestions come
from simple, explainable rules: same category, same era, the bidder's watchlist
preferences, then lots closing soon. This is **not** machine learning.

### 2.10 Feature labs (design prototypes)

`/admin/labs` contains clickable prototypes of planned features: multi-currency
display (USD / VND), soft-close anti-sniping, proxy (maximum) bidding and
forecasting analytics. They use built-in sample data and never touch the
database. Designs are in [docs/advanced-features-design.md](docs/advanced-features-design.md).

![Feature labs](docs/screenshots/26-admin-labs.png)

---

## 3. Tech stack

| Layer | Technology | Why |
|---|---|---|
| Backend | Go 1.26, standard `net/http` | One small, fast binary with no web framework |
| Database | PostgreSQL 18 via `pgx/v5` | Transactions keep bids, results, orders and payments consistent |
| Frontend | React 19, TypeScript, Vite | Typed single-page app with fast builds |
| UI | Tailwind CSS 4, shadcn/ui, Recharts | Accessible components and analytics charts |
| Real-time | Server-Sent Events for live auctions (falls back to polling); WebSocket (gorilla/websocket) for the retained classroom game | Prices and leading/outbid status update without reloading |
| Packaging | Frontend embedded into the Go binary (`CGO_ENABLED=0`) | One file to run; Node is only needed to build |

---

## 4. Architecture

```mermaid
flowchart LR
    B[Browser<br/>React SPA] -- HTTPS / JSON --> S
    subgraph S[Go server — single binary]
        direction TB
        API[HTTP API<br/>internal/api] --> CAT[catalog]
        API --> ACC[account<br/>sessions, watchlist]
        API --> AUC[auction<br/>bids, closing worker]
        API --> ORD[order<br/>checkout, fulfilment]
        API --> ANA[analytics &<br/>recommendation]
        UI[Embedded frontend<br/>web/dist]
    end
    S -- pgx --> DB[(PostgreSQL 18<br/>one schema per environment)]
```

- **One writer per schema.** The server holds an exclusive lock on its database schema, so two servers can never write the same data.
- **Commit before publish.** A bid, a closing result or a payment is reported as successful only after the database transaction commits.
- **Background closing worker.** It checks every 250 ms, opens and closes lots on schedule, and creates the winner's order in the same transaction as the sold result.
- **Admin capability.** Every admin page and API checks a secret cookie, which is set by opening the admin link. Bidder accounts can never reach the admin area.

| Folder | Contents |
|---|---|
| `cmd/server/` | Program entry point, startup and maintenance commands (demo seeding, reset) |
| `internal/api/` | HTTP handlers and the admin gate |
| `internal/catalog/` | Auction events, lots, publishing, demo seed data |
| `internal/account/` | Registration, login, sessions, watchlist |
| `internal/auction/` | Timed bidding and the closing worker |
| `internal/order/` | Winner orders, checkout, simulated payment, fulfilment |
| `internal/analytics/`, `internal/recommendation/`, `internal/interaction/` | Dashboard metrics, rule-based suggestions, page-view tracking |
| `internal/store/` | PostgreSQL connection, migrations (`001`–`006`) |
| `internal/game/`, `internal/hub/` | The original classroom auction game (kept, separate from the shop) |
| `web/src/` | React frontend: `routes/storefront`, `routes/account`, `routes/admin` |

---

## 5. Quick start

### Prerequisites

- **Go 1.26+** and **Node.js 20+** (Node is only needed to build the frontend)
- **PostgreSQL 18**, including the `psql` and `pg_restore` client tools
- macOS or Linux with `make`

### Step 1. Start PostgreSQL

If PostgreSQL 18 is already installed, create a database and skip ahead:

```sh
createdb auctionlab
```

**Fastest option with Docker:**

```sh
docker run -d --name auctionlab-pg18 \
  -e POSTGRES_USER=auctionlab -e POSTGRES_PASSWORD=auctionlab -e POSTGRES_DB=auctionlab \
  -p 127.0.0.1:5432:5432 postgres:18
```

### Step 2. Point the app at the database

```sh
# Local install (replace <you> with your OS user name):
export DATABASE_URL='postgres://<you>@localhost:5432/auctionlab?sslmode=disable'

# Or, if you used the Docker command above:
export DATABASE_URL='postgres://auctionlab:auctionlab@localhost:5432/auctionlab?sslmode=disable'
```

### Step 3. Load the demo data and run (recommended)

The repository includes a **pre-played demo snapshot**,
`demo/auctionlab_demo.dump`. It contains published auctions, real bid history,
sold and unsold lots, paid and unpaid orders and shipped or picked-up
deliveries, so every screen above has data immediately.

```sh
make demo-restore   # loads the snapshot into schema auctionlab_demo_submission
make demo           # builds the app and serves the demo on http://localhost:8080
```

> **Using Docker without local PostgreSQL tools?** Restore from inside the
> container instead of running `make demo-restore`:
>
> ```sh
> docker exec -i auctionlab-pg18 pg_restore -U auctionlab -d auctionlab \
>   --no-owner --no-privileges < demo/auctionlab_demo.dump
> make demo
> ```

Then open:

| What | URL |
|---|---|
| Storefront | <http://localhost:8080> |
| Admin area (opens the admin gate) | <http://localhost:8080/admin?k=auctionlab-demo-admin> |
| Health check | <http://localhost:8080/api/health> |

**Demo accounts.** These passwords exist only in the demo data.

| Role | Email | Password |
|---|---|---|
| Bidder 1 (Linh) | `bidder-one@demo.auctionlab.test` | `bidder-one-demo-2026` |
| Bidder 2 (Minh) | `bidder-two@demo.auctionlab.test` | `bidder-two-demo-2026` |
| Operator account | `operator@demo.auctionlab.test` | `operator-demo-2026` |

> The admin area is opened by the **admin link** above, not by the operator
> account. In demo mode the key is fixed to `auctionlab-demo-admin`. In normal
> mode the server prints a random admin link in its console at startup.

### Other ways to run

| Command | What it does |
|---|---|
| `make demo-fresh` | Seeds a **new, empty-history** demo schema (`auctionlab_demo_fresh`) with 3 draft events, 18 lots and the 3 demo accounts, then starts the server. Use this to run an auction from scratch. |
| `make run` | Builds and runs on the normal `auctionlab` schema (empty on first start) |
| `make dev` | Development mode: Go API on `:8080` plus the Vite dev server on `:5173` with hot reload |
| `FORCE=1 make demo-restore` | Replaces the demo snapshot schema with a clean copy |

---

## 6. Demo walkthrough (≈10 minutes)

After `make demo-restore && make demo`:

1. **Browse as a visitor.** Open <http://localhost:8080>, then **Auctions → Objects of Time**. Open a lot and look at the gallery, condition notes and bidding panel.
2. **Bid as Bidder 1.** Sign in as `bidder-one@…` (person icon, top right). On an *Objects of Time* lot, place a bid at the suggested minimum. The banner turns green: **You are leading**.
3. **Outbid as Bidder 2.** In a private or second browser window, sign in as `bidder-two@…` and bid on the same lot. Bidder 1's window updates live to **You have been outbid**.
4. **See a finished auction.** *Analog Memories* has already closed. Open a sold lot to see the result, and *Leather travel album* to see an unsold lot with no bids.
5. **Pay for a win.** As Bidder 1, open the **cart icon (Unpaid wins)**. *Ceramic picture frame* is waiting. Check out with shipping or pickup, review and confirm.
6. **Fulfil as operator.** Open the [admin link](http://localhost:8080/admin?k=auctionlab-demo-admin) → **Orders**. Advance the new order to *preparing → shipped*, with any carrier and tracking text.
7. **Track as buyer.** Back as Bidder 1, open **Orders** and select the order. The timeline shows each step.
8. **Review analytics.** In the admin area, open **Analytics** and switch between events.
9. **Run a brand-new auction (optional).** In admin → **Auctions**, open the draft *The Listening Room*. Set the opening a few minutes ahead and short closing times, then publish it and bid with both accounts. The server closes each lot on schedule and creates the winner's order automatically.

> The snapshot was recorded on 15 Nov 2026. *Objects of Time* stays open for
> bidding until **22 Nov 2026**. After that its lots close automatically when
> the server starts, and bidding works best on a newly published event (step 9)
> or with `make demo-fresh`.

---

## 7. Running tests

Tests that use the database need a **separate, disposable test database**.
Each test creates its own throwaway schema and drops it afterwards.

```sh
createdb auctionlab_test
export TEST_DATABASE_URL='postgres://<you>@localhost:5432/auctionlab_test?sslmode=disable'

make test        # Go tests + frontend tests (Vitest) + lint
make test-race   # Go tests with the race detector
```

Database tests fail with a clear message if `TEST_DATABASE_URL` is missing.
They are never skipped silently.

---

## 8. Project structure and further docs

| Document | Contents |
|---|---|
| [docs/operations.md](docs/operations.md) | Detailed operator notes: closing and recovery rules, checkout edge cases, demo reset |
| [docs/postgresql.md](docs/postgresql.md) | Database roles, migrations and upgrades, backups, demo schema preparation and reset |
| [web/README.md](web/README.md) | Frontend structure and the full API contracts |
| [docs/advanced-features-design.md](docs/advanced-features-design.md) | Designs for currency, soft close, proxy bidding and analytics v2 |
| [ROADMAP.md](ROADMAP.md) | Planned work |
| `internal/*/README.md` | Per-module rules (accounts, auctions, orders, analytics, recommendations) |
| `openspec/changes/archive/` | Specifications and verification reports for each delivered change |

---

## 9. Limitations and scope

- **Payments and delivery are simulated.** No payment gateway or carrier is connected.
- **Recommendations are rule-based**, not trained machine learning.
- **One server process per database schema.** Horizontal scaling is out of scope.
- Feature labs (multi-currency, soft close, proxy bidding, forecasting) are **prototypes only** and are not active in real auctions.
- Two accessibility checks (exact 200% browser zoom and reduced-motion) are recorded as still open in the verification reports.

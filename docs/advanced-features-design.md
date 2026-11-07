# AuctionLab — Advanced Feature Designs

Status: design plus UI prototypes (2026-11-07). None of these are implemented in
the backend. Features 1, 2, 3 and 8 have clickable, fixture-only prototypes at
**`/admin/labs`** (see [Prototypes](#prototypes-adminlabs)). Each feature is
scoped so it could become its own OpenSpec change after the existing Changes 1–8.

Ranked by value for a vintage, single-seller, timed English auction:

| # | Feature | Industry name | Effort | Why it matters here |
|---|---|---|---|---|
| 1 | USD + VND for international and local bidders | **Multi-currency display** (settlement currency + display currency) | M | Local bidders think in đồng, international ones in dollars |
| 2 | Bids near the close extend the clock | **Anti-sniping / soft close / extended bidding** | S | Timed lots close one after another, so last-second sniping is the main fairness risk |
| 3 | "Bid up to my maximum for me" | **Proxy bidding / absentee max bid** | M | Bidders don't have to watch the countdown |
| 4 | Hidden floor + house commission | **Reserve price** + **buyer's premium** | S–M | Standard auction-house economics |
| 5 | Outbid / closing-soon / you-won alerts | **Bid notifications** | S | Turns the watchlist into re-engagement |
| 6 | Shill-bid and anomaly flags in admin | **Bid integrity monitoring** | M | Credibility; makes the analytics page actionable |
| 7 | Estimates and past results | **Pre-sale estimate** + **prices realised** archive | S | Vintage buyers anchor on comparables |
| 8 | Analytics v2 | Time series, bid timing, estimate accuracy | M | Builds on the shadcn dashboard |

---

## Prototypes (`/admin/labs`)

Build order: **currency → soft close → proxy bidding → analytics v2**. Each one
is a tab on one admin page (`#currency`, `#soft-close`, `#proxy`, `#analytics`
deep-link to a tab). Every screen has a "Prototype · fixture data" label and
English/Vietnamese copy. The prototypes don't read or write the catalogue, bids,
orders or the database. The rules each UI demonstrates are pure functions in
`web/src/lib/labs.ts` with unit tests in `labs.test.ts` (13 cases), so the same
logic can move to the Go backend later.

| Tab | File | What you can do |
|---|---|---|
| 1 · Currency | `routes/admin/labs/CurrencyLab.tsx` | Switch display currency (follow language / USD / VND); type a bid in ₫ and see the USD amount it rounds up to; publish a new operator rate and watch the lot card change while the receipt keeps its snapshot rate |
| 2 · Soft close | `routes/admin/labs/SoftCloseLab.tsx` | Pick the extension window (1/2/5 min) and cap; skip to the final 30 s; simulate bids and watch the countdown extend, the cap meter fill and the extension log grow |
| 3 · Proxy bidding | `routes/admin/labs/ProxyLab.tsx` | Set your private maximum and a rival's; see the price resolve to one increment over the lower max, the Leading/Outbid chip change, and "auto" bids in the public history |
| 4 · Analytics v2 | `routes/admin/labs/AnalyticsLab.tsx` | Headline row (realised, projected range, 7-day bids with ▲/▼ change, final-2-minute share); revenue forecast with +20–30 / 30–40 / 40–50% uplift; bids over time (7/30-day toggle), bid-timing histogram, sell-through by category, hammer-vs-estimate band plot, and winning value in USD or VND at each order's snapshot rate. Each chart lists the backend aggregate it needs |

### Screen layouts

Every bidding prototype uses a **lot stage**, laid out the same way as the real lot
page: the seed catalogue picture (`/site-assets/catalog/lot-NN.svg`, plus its
detail thumbnail) on the left, and the price and controls on the right. Supporting
panels (receipt, rate admin, logs, rules) sit underneath in two columns. The
fixture lots are real seed lots: 02 Silver rangefinder study, 07 Field watch with
canvas strap, 17 Belt drive turntable. The sketches show the right-hand column;
the picture sits to its left.

**1 · Currency**
```
[Follow language (USD)] [$ USD] [₫ VND]                  1 USD = 25.380 ₫ · 06 Nov 2026
┌ Lot card ─────────────┐ ┌ Bid entry ──────────────┐ ┌ Receipt (rate snapshot) ─┐
│ Silver rangefinder    │ │ Min ≈ 4.442.000 ₫  +$5  │ │ Hammer          $150.00  │
│ ≈ 4.315.000 ₫         │ │ [ 4.400.000        ₫ ]  │ │ Delivery         $15.00  │
│ charged as $170.00 USD│ │ Will submit $175.00 USD │ │ Total charged   $165.00  │
└───────────────────────┘ │ rounded up to increment │ │ Shown as ≈ 4.144.800 ₫   │
                          └─────────────────────────┘ │ at 25.120 ₫/$ (18 Oct)   │
┌ Exchange rate (operator) ┐  ┌ Rules ─────────────┐  └──────────────────────────┘
│ [25380] [Publish rate]   │  │ 1. USD cents only… │
│ 25.380/$ operator  LIVE  │  │ 2. …               │
│ 25.120/$ seed      ORDER │  └────────────────────┘
```

**2 · Soft close**
```
Window [1 min][2 min][5 min]   Cap [+5][+10][+30 min]   Reset
┌ Belt drive turntable ─────────────── EXTENSION WINDOW ┐ ┌ Extension log ───────┐
│ CLOSES IN                              CURRENT BID     │ │ 14:02:41 00:18→02:00 │
│ 01:47                                  $430.00 · 8 bids│ │          +01:42      │
│ Extended ×1 · +01:42                                   │ └──────────────────────┘
│ Extension cap used  ▓▓░░░░░░░░  +01:42 / +10:00        │ ┌ Rules ───────────────┐
│ [Simulate a bid]  [Skip to final 30 s]                 │ │ 1–4 …                │
└────────────────────────────────────────────────────────┘ └──────────────────────┘
```

**3 · Proxy bidding**
```
┌ Field watch with canvas strap ─────────────── Reset ┐ ┌ Public bid history ┐
│ CURRENT BID                     ┌ LEADING ──────────┐│ │ You   AUTO $290.00 │
│ $290.00                         │ Auto-bidding up to││ │ Rival      $280.00 │
│ Next min $300.00 · +$10.00      │ $350.00           ││ │ You        $200.00 │
│ Your maximum (private) [    ]   Rival maximum [    ] ││ └────────────────────┘
│ [Set maximum]                   [Rival bids]         ││
└──────────────────────────────────────────────────────┘
```

**4 · Analytics v2**

Colour does one job per chart. **Blue** is the measure and the emphasised mark;
the same blue at low opacity is everything else. **Orange** is what needs
attention: the sniping spike and below-average categories. **Aqua** means above
estimate, and green/red arrows show change against the previous period. Every
highlight also has a text label, so the meaning never depends on colour alone.

```
[REALISED $1,192] [PROJECTED $2,089–$2,158] [BIDS 7d 135 ▲10.7%] [FINAL-2-MIN 37.6%]
┌ Revenue forecast                       [+20–30%][+30–40%][+40–50%] ┐
│ $ ─────solid realised───┊- - dashed forecast ░band░ ● $2,123.50     │
│                       Today      Open lots: Oak mantel clock $140 → $189 … │
└────────────────────────────────────────────────────────────────────┘
┌ Accepted bids over time  [7d][30d] ─────────┐ ┌ Winning value [$][₫] ┐
│ area chart, 30 daily points                 │ │ ≈ 17.987.420 ₫       │
└─────────────────────────────────────────────┘ │ per-order × rate     │
┌ When bids arrive (min before close) ┐ ┌ Sell-through by category ┐
│ >60 30–60 10–30 5–10 2–5  <2 (tall) │ │ Watches ▓▓▓▓▓▓▓ 75%      │
└─────────────────────────────────────┘ └──────────────────────────┘
┌ Hammer vs estimate   ● below ● within ● above ─────────────────────┐
│ [img] Silver rangefinder ─[██]─● $205.00  $120–$180 · above     │
└─────────────────────────────────────────────────────────────────────┘
```

Screenshots of each tab (captured 2026-11-07, 1400 px viewport, English) are kept
with the course report, not in this repository.

---

## 1. Multi-currency display (USD ⇄ VND)

### What it's called

It's more than a conversion, but it isn't full multi-currency either. The parts:

- **Settlement (base) currency**: the currency money is stored in and charged
  in. Here: **USD**, integer cents in every `*_minor` column. This does not change.
- **Display (presentment) currency**: what the user *sees*. USD or VND, a user
  preference.
- **Indicative conversion**: the VND figure is labelled *approximate* ("≈").
- **FX rate snapshot**: when an order is created, the rate is frozen onto the order
  so the receipt never changes later.

Real platforms call this **multi-currency display** or **currency localization**.
eBay's "approximately ₫…" under a listing is this exact pattern. Two nearby terms
are *not* this:

- **Dynamic Currency Conversion (DCC)** is a card-payment term: the payment
  terminal charges in the cardholder's currency.
- **Multi-currency settlement** means you actually charge in VND. That needs a
  VND-denominated ledger and is out of scope.

### Current state (uncommitted work in `web/src/lib/catalog-money.ts`)

- Currency is **tied to language**: `lang === 'vi'` shows đồng and `en` shows USD.
- The rate is a **hard-coded constant** (`VND_PER_USD = 25_000`). One cent is
  exactly 250 ₫, so there is no rounding.
- The VI bid form accepts đồng and rejects amounts not divisible by 250.

This is a sound first step. Three problems as a product feature:

1. A Vietnamese speaker abroad may want USD, and a foreigner in Vietnam may want
   VND. Language ≠ currency.
2. A frozen rate looks unrealistic and can't be explained in a receipt.
3. A VND amount without an "≈" reads as the charge amount, which can mislead
   bidders.

### Design

**Rules**

| Rule | Decision |
|---|---|
| Source of truth | USD cents. Bids, increments, reserve, orders, and analytics are all stored in USD. |
| Display currency | `USD` \| `VND`, chosen independently of language. Default: `VND` if lang=vi, else `USD`. |
| Rate | `fx_rates` row: `base='USD', quote='VND', rate (integer ₫ per $1), source, effective_at`. Entered or approved by the operator. Fetching at runtime from a third party isn't required, which matches the offline/embedded constraint. |
| Rounding | VND has 0 minor units (ISO 4217). Display rounds to the nearest 1,000 ₫ for prices and 1 ₫ for receipts. |
| Labelling | Primary figure in the display currency, with the other currency as a secondary line. VND always carries "≈" and "charged in USD". |
| Bid entry in VND | Allowed. The server converts with the current rate, then **rounds UP to the next valid USD increment** and shows a confirm step: "Your bid: $175.00 (≈ 4.375.000 ₫)". The bidder never pays less than they typed. |
| Order snapshot | On order creation, store `fx_rate_id` and `display_total_vnd` so the receipt shows the rate in force when they won. |
| Analytics | Report in USD. Optional VND view converts **each order at its own snapshot rate**, never today's rate, and labels the result as such. |

**Data model (additive migration 7)**

```sql
CREATE TABLE fx_rates (
  id            BIGSERIAL PRIMARY KEY,
  base          CHAR(3) NOT NULL CHECK (base = 'USD'),
  quote         CHAR(3) NOT NULL CHECK (quote = 'VND'),
  rate_whole    BIGINT  NOT NULL CHECK (rate_whole BETWEEN 1000 AND 1000000), -- ₫ per $1
  source        TEXT    NOT NULL,          -- 'operator' | 'seed' | e.g. 'SBV reference'
  effective_at  BIGINT  NOT NULL,          -- ms epoch
  created_by    TEXT    NOT NULL
);
ALTER TABLE commerce_accounts ADD COLUMN display_currency CHAR(3)
  CHECK (display_currency IN ('USD','VND'));           -- NULL = follow language
ALTER TABLE commerce_orders ADD COLUMN fx_rate_id BIGINT REFERENCES fx_rates(id);
```

API: `GET /api/catalog/fx` → `{ base, quote, rate_whole, effective_at }`.
Admin: `POST /api/admin/fx` behind the catalog capability.

**UI**

```
Header:  [EN | VI]   [$ USD ▾]          ← currency switcher next to the language switcher

Lot card:     4.375.000 ₫
              ≈ · charged as $175.00 USD

Bid panel:    Your bid  [ 4.400.000 ] ₫
              = $176.00 USD after rounding up to the next $1 increment
              Rate 25.000 ₫/$ · updated 06 Nov 2026

Receipt:      Total charged   $190.00 USD
              Shown as        ≈ 4.750.000 ₫ at 25.000 ₫/$ (rate of 06 Nov 2026)
```

**Edge cases**

- Rate changes during a live lot: the displayed VND for the current price
  updates, but the USD standing doesn't. Show "rate updated" quietly and don't
  re-sort lists.
- Missing rate: hide VND and show USD only. Never fall back to a guessed constant.
- Accessibility: screen-reader text says "approximately 4,375,000 dong, charged as
  175 US dollars".

**Pitch line:** *"One settlement currency, two presentment currencies, auditable
FX snapshots."*

---

## 2. Anti-sniping (soft close)

- **Rule:** if an accepted bid lands within the final `extend_window` (default
  2 min) of a lot's close, the close moves to `bid_time + extend_window`. Cap the
  extensions with `max_close` (e.g. +30 min) so a lot can't run forever.
- **Why:** lots close one after another. Without this, the winner is whoever has
  the lowest latency, not the highest valuation.
- **Data:** `commerce_lot_auctions.extend_window_ms`, `commerce_lot_auctions.max_close_at`; each extension is an
  event row (`lot_close_extended`) so realtime clients and analytics can see it.
- **UI:** countdown flashes "Extended +2:00", and the lot page shows
  "Soft close: bids in the last 2 min extend the lot".
- **Correctness:** extension happens in the same transaction as bid acceptance.
  The close scheduler reads the committed close time, which fits the existing
  "commit before publish" rule.

## 3. Proxy bidding (max bid)

- The bidder submits a private **maximum**. The system bids the minimum needed to
  lead, one increment at a time, up to that max.
- If two proxies tie, the earlier one wins at the tied amount.
- **Privacy:** the max is owner-private, like checkout data. Public bid history
  shows "auto-bid" amounts only.
- **Pairs with soft close:** proxy bids also trigger extensions.
- **UI:** "Set max bid" toggle in the bid panel, and an "Auto-bidding up to
  $250 · leading at $180" standing chip.

## 4. Reserve price + buyer's premium

- **Reserve:** hidden minimum. Show "Reserve not met" or "Reserve met", never the
  value. Closing below reserve gives `unsold (reserve not met)`, a new terminal
  reason that analytics can split out.
- **Buyer's premium:** a % added on top of the hammer price (e.g. 15%), shown
  before bidding. Order total = hammer + premium + delivery. Analytics gains
  "premium revenue".

## 5. Bid notifications

- Events: outbid, watched lot closing in 15 min, won, payment reminder.
- Channel: an in-app inbox first (no external email dependency). Email/Zalo is
  future work.
- **Data:** `notifications(account_id, kind, lot_id, created_at, read_at)`.
- Fed by the existing realtime pipeline after commit.

## 6. Bid integrity monitoring

Rule-based flags shown in admin (not ML, consistent with `rules-v1`
recommendations):

- The same bidder pair repeatedly bids against each other across lots and one
  never wins (possible shill).
- A brand-new account places a large jump bid in the final minute.
- Bid velocity spikes (e.g. >10 bids per minute on one lot).

Output: an "Integrity" card on the analytics page with flagged lots. Flags are
facts with reasons, and the operator decides what to do.

## 7. Pre-sale estimates + prices realised

- Lots carry an estimate range (`estimate_low_minor`, `estimate_high_minor`),
  displayed as "Estimate $150–$250".
- After close, a public **Prices realised** page lists hammer prices per event.
- Analytics: **hammer / estimate ratio** (below, within, or above estimate),
  which shows how well the house prices its items.

## 8. Analytics v2 (builds on the shadcn dashboard)

| Chart | Form | Needs from backend |
|---|---|---|
| **Revenue forecast** | Realised revenue (solid) + forecast midpoint (dashed) + uplift band, "Today" marker, list of open lots with predicted hammer | Daily realised hammer + open lots' current bid and close time. Rule: predicted hammer = current bid × (1 + 30–40%), adjustable 20–30 / 30–40 / 40–50%. Rule-based (`predictHammer`, `forecastRevenue` in `lib/labs.ts`), not ML; later calibrate the uplift from past results (hammer ÷ bid one day before close) |
| Bids and winning value over time | Area/line, date-range control | Daily buckets (`date_trunc('day', created_at)`) |
| When bids arrive relative to close | Histogram (minutes before close) | `close_at - bid_at` per accepted bid |
| Price vs estimate | Dot plot per lot (low–high band + hammer dot) | Feature 7 |
| Bidder concentration | "Top 5 bidders share of bids" stat tile | Per-account counts (aggregate only) |
| Category performance | Bar: sell-through by category | Group by `catalog_lots` category |
| Currency view | USD/VND toggle using order snapshot rates | Feature 1 |

Privacy stays as today: aggregates only, and no per-person data on the dashboard.

---

### Suggested order

1 (currency) → 2 (soft close) → 3 (proxy bidding) → 8 (analytics v2) → the rest.
Currency and soft close are the best demo value for the effort. Proxy bidding and
analytics v2 produce the most interesting data to show.

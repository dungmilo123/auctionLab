# Auction analytics

`GET /api/admin/analytics?event=all&dataset=all` is capability-only (including
unknown subtree paths), no-store, and returns aggregate data only. No commerce
account is required; an account admin role does not grant capability access.
Strict filters: `event=all|published-slug`, `dataset=all|demo|non-demo`; omitted
means all; empty/duplicate/unknown/malformed queries are 422, draft/missing event
404, inconsistent sources/storage 503. There is no date filter, raw export or
analytics SSE.

One bounded read-only repeatable-read transaction selects published lots in
published events, validates required sold-result/order/receipt bindings and
amounts, calculates independent bid/result/payment aggregates and collection
coverage, and stamps `meta.as_of`. Zero-cohort published events remain in
slug C-order. All decimal integer counts/totals are strings, including zero;
sums use PostgreSQL numeric, not a per-order cap. Sell-through is null or exact
`{sold,terminal}` strings. Browser formatting uses BigInt and rounds only the
presentation percentage.

- `lots`: published cohort lots, including uninitialized executions.
- `bids`: accepted bid rows. `bidders`: distinct accepted-bid account IDs.
  Global distinct counts are computed across events; event bidder counts are
  not additive. Demo/non-demo actor counts partition accepted participation.
- `sold`, `unsold`, `terminal`: committed outcomes; terminal = sold + unsold.
  `pending` = lots - terminal, including elapsed but unreconciled schedules.
  Sell-through = sold / terminal, null with denominator zero.
- `winning_minor`: sold-result winning amounts, including unpaid wins.
- `paid_orders`: one confirmed simulated receipt per order.
- `unpaid_sold_orders`: sold orders awaiting payment.
- `paid_winning_minor`, `paid_fees_minor`: paid order components.
  `simulated_paid_minor`: stored receipt totals = these two components summed.
  Every receipt is simulated; replay/fulfillment changes no financial totals.

A demo cohort lot means lot.demo OR event.demo. A demo actor bidding on a
non-demo lot stays in the non-demo cohort, with actor demo reported separately.
Late payments remain attributed to the original event. No contact JSON is
selected. Interaction counts group kind/provenance and are separate from full
lifetime commerce facts: views are observations, backfills are durable history,
current watchlists are snapshots, and collection before its baseline is unknown.
The verification fixture uses A=2 and B=2 bidders, six accepted bids, two global
bidders, 3/4 sold, 35000 winning cents and 16500 simulated paid cents. It tests
implementation arithmetic; it provides no recommendation effectiveness evidence.

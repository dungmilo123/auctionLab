# Proposal

## Why

Change 5 records durable auction winners, but a winner cannot yet complete the roadmap's purchase flow. Change 6 connects each sold lot to an owned order and an explicitly simulated payment, with exact published delivery charges and repeat-safe confirmation.

## What Changes

- Automatically create one awaiting-payment order per sold commerce result, including already-recorded winners on upgrade; unsold lots create none.
- Freeze public lot identity/content needed for purchase review, winning bid/account/amount, USD currency, demo provenance, and published delivery/pickup terms on the order.
- Add `/account/wins` for unpaid wins and `/account/checkout/:orderId` for a single order's contact, fulfillment choice, review, payment simulation, and durable paid confirmation.
- Calculate totals on the server: winning amount plus the published flat shipping fee for delivery, or zero delivery charge for pickup. No taxes, buyer premium, discounts or combined checkout.
- Offer `demo-card` and `demo-bank-transfer` as labeled simulation choices without collecting card, bank-account, or other real payment credentials.
- Atomically record at most one simulated payment per order and transition awaiting-payment to paid; repeated or concurrent confirmation cannot duplicate either record.
- Replace obsolete checkout-unavailable copy and connect won participation to owned checkout, while keeping `/account/orders`, `/admin/orders`, fulfillment timelines, reset tools and order administration in Change 7.

## Capabilities

### New Capabilities

- `winner-checkout`: Automatic orders from sold results, immutable purchase snapshots, private unpaid-win discovery, single-order checkout and exact fulfillment totals.
- `demo-payments`: Owner-authorized, retry-safe simulated confirmation and durable payment receipts without real payment processing.

### Modified Capabilities

- `timed-auction-bidding`: Sold closure creates its order atomically; personal won participation exposes an owned checkout link and confirmed payment state.
- `auction-event-publishing`: Preserve publication/execution separation while permitting result-driven order creation by commerce closure.
- `postgresql-persistence`: Add migration 4 for orders/payments, upgrade and backfill behavior, and writer/commit safety for checkout.
- `website-navigation`: Recognize private wins and checkout routes, safe login return, and accurate available-versus-preview notices.
- `auction-discovery`: Keep public purchase disclosure and browsing boundaries accurate when winner checkout becomes available privately.
- `catalog-administration`: Update availability wording without adding order operations or changing capability authorization.

## Impact

Backend: `internal/store/migrations/004_winner_checkout.sql`, migration registration/readiness, `internal/store/auction.go` closure/reconciliation/participation, a new commerce-order domain/repository, private handlers under the existing `AccountAPI` namespace, and `cmd/server/main.go` wiring. Extend the SQLite import target-occupancy guard for the new tables; keep migrations 1–3 and legacy monetary units unchanged.

Frontend: account routes/layout/titles and safe returns, account-owned request state, won participation and lot-panel links, new checkout/wins views, exact money display and EN/VI copy. No new external service or dependency is planned. PostgreSQL remains separately required; normal startup provisions no credentials. Update active operator documentation and `AGENTS.md` only during apply to reflect schema 4 and the implemented feature boundary; preserve archived evidence and its unverified Change 4/5 accessibility checks.

Out of scope: real charges/carriers, abandonment enforcement, refunds, cancellations, fulfillment updates, consolidated payments, cart/Buy Now, seller payouts, analytics and ML.

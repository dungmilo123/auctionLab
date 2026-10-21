# Proposal

## Why

Winners can pay through Change 6, but buyers cannot browse their purchase history and operators cannot carry an order through delivery or pickup. Change 7 completes the roadmap's purchase-to-receipt demonstration and provides a bounded way to prepare another rehearsal.

## What Changes

- Replace `/account/orders` with owned history and add `/account/orders/:orderId` for purchase details, simulated receipt, manual shipping information and a durable status timeline. Existing wins and checkout URLs continue working.
- Activate `/admin/orders` with status/method filters, selected-order inspection and deliberate fulfillment transitions. Dedicated admin APIs use the existing capability cookie; account APIs remain owner-only. Explicitly replace older spec statements that capabilities grant no order visibility with this narrow operational exception.
- Support shipping `awaiting_payment → paid → preparing → shipped → completed` and pickup `awaiting_payment → paid → ready_for_pickup → completed`. Payment remains immutable; fulfillment cannot bypass payment, skip stages, switch methods or reopen completed orders.
- Add additive PostgreSQL migration 5 for fulfillment progress/history and demo-environment metadata. Preserve migrations 1–4, purchase terms, payment receipts, bidding results and legacy game behavior.
- Provide stopped-writer preparation/reset commands restricted to an explicitly created disposable demo schema, as selected by the user. Reset removes that schema's rehearsal data and reseeds accounts/draft catalog content; normal schemas are refused. Publication, bidding, payment and fulfillment still require the normal deliberate workflow.
- Keep EN/VI, mobile/desktop, keyboard, actual 200% browser zoom and reduced-motion acceptance explicit. Earlier archived acceptance gaps remain historical gaps.

Out of scope: real payment/carrier integrations, automatic shipping updates, notifications, cancellations/refunds/returns, unpaid-winner enforcement, order merging, arbitrary status editing, selective deletion in operational schemas, analytics/recommendations and ML.

## Capabilities

### New Capabilities

- `order-history`: Owner-scoped order collection/detail and accessible purchase/timeline navigation.
- `demo-fulfillment`: Capability-authorized operations, method-specific transitions, manual shipping data and durable progress.
- `demo-rehearsal`: Explicit isolated demo preparation/reset with a documented write scope and repeatable fresh schedules.

### Modified Capabilities

- `winner-checkout`: Preserve owner-only checkout while exposing history/timeline links and the distinct admin namespace.
- `demo-payments`: Initialize fulfillment atomically with payment and define the bounded admin projection exception.
- `bidder-accounts`: Clarify capability-authorized order operations without granting account impersonation or general bidder-private access.
- `catalog-administration`: Replace Change 7 preview notices and define the narrow order-management boundary.
- `website-navigation`: Activate history/admin routes and add exact owner-order detail URLs.
- `auction-discovery`: Link the implemented private post-purchase experience without leaking purchase data publicly.
- `postgresql-persistence`: Add schema-5 upgrade, fulfillment commit/restart safety and importer occupancy coverage.

## Impact

Go order/store/API packages, server route registration and maintenance dispatch; React route parsing, private account views, admin order workspace, DTO validation and EN/VI copy. Update active README/operator/API documentation during implementation. Use the existing PostgreSQL writer lease, same-origin guards, local assets and frontend-first embedded build; no new service or external integration is planned. Planning creates only this change's artifacts and does not run a demo reset or edit application code.

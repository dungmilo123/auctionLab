# Commerce timed auctions

The auction repository uses the existing leased writer connection. After cancellation-aware queue waiting it validates the commerce session, resolves a published target, and checks an accepted `(account_id, request_id)` receipt. Fresh intents initialize/lock the auction row, sample time after lock waiting, revalidate session expiry and enforce `opens_at <= now < closes_at`, current-leader refusal, then the exact minimum. Time is the validation instant; commit may finish after close. No game engine, budget, watchlist prerequisite, order or payment is involved.

First minimum equals opening cents (including zero). A 12550 opening with a 500 increment accepts 12550, then 14001 (a jump); the next minimum is 14501. There is no increment grid. Maximum is 1000000000000 cents. Exhaustion produces null minimum and `amount_limit_reached`. A late bid is refused even if the worker still presents closing. Rejections append nothing and reserve no request ID.

Accepted bids, sequence/count/leader/revision commit atomically. Request IDs are lowercase UUID strings, scoped to the session's account. Identical accepted retries return the original receipt after restart/closure (HTTP 200 instead of new acceptance 201); changed lot/amount conflicts. Every replay still requires a current session. Pre-commit failure rolls back; unknown commit/transport outcomes are not success. Reconcile the private lot's accepted request IDs or explicitly retry the same intent and same ID; never automatically create a fresh ID for an unknown outcome. Lost writer ownership requires process restart, never lease reacquisition.

## HTTP

All responses use `Cache-Control: no-store`, `{data,meta:{server_time}}` for success and structured `{error:{code}}` for failure. Available conflict snapshots use `data.snapshot` plus server-time metadata.

- `GET /api/auctions/lots/{lot_id}`: coherent published-only snapshot; current price is null before bidding. Separate execution states are pending/scheduled/opening/open/closing/sold/unsold. Revision changes only on committed transitions/bids. Catalog published DTOs, schedules and opening prices do not change.
- `GET /api/auctions/lots/{lot_id}/bids?before_sequence=6`: newest-first page, at most 50, exclusive sequence cursor. `before_sequence` in a page is the next cursor or null. Stable per-lot aliases are `Bidder N`; same-time bids still order by sequence.
- `GET /api/auctions/lots/{lot_id}/stream`: read-only SSE full `snapshot` events (revision IDs), initially and on revision/presentation changes. One-second samples, ten-second server-time heartbeat, 64 streams, five-second write bound. Reconnect always reads a full snapshot; Last-Event-ID never requests replay. No reads initialize/close an auction, retain a transaction between writes or hold the writer lease. Shutdown cancels streams before waiting for HTTP handlers.
- `POST /api/account/bids`: current session, `Content-Type: application/json`, `X-AuctionLab-Account: 1`, same-origin boundary, strict 8 KiB JSON, e.g. `{"lot_id":"catalog-lot-id","amount_minor":14001,"request_id":"00000000-0000-0000-0000-000000000001"}`. Identity comes only from the session. Response data contains `receipt` (lot ID, own accepted bid and replay flag) plus fresh `snapshot` when available.
- `GET /api/account/bids`: private unique-lot participation, ordered by latest own acceptance time descending, then lot ID. Only accepted participation appears.
- `GET /api/account/bids/{lot_id}`: own accepted bids/request IDs, latest own bid, current price/revision and not_participating/leading/outbid/closing/won/lost. Checkout is false. The full account namespace guard applies, including unknown paths.

Status: 201 new acceptance; 200 read/replay; 400 malformed/unknown fields or invalid request ID; 401 invalid session; 403 rejected origin/header; 404 indistinguishable missing/draft; 409 minimum/window/leader/request conflict; 415 wrong content type (existing account boundary); 422 invalid amount; 503 unavailable or `commit_unknown`. Streams at capacity send Retry-After 2. Error messages never echo SQL, secrets or caller values.

Public fields are explicit DTOs: lot, revision, state, schedule, USD amounts/count, sequence/time/alias, and terminal public winner reference. They exclude account IDs, names, emails, credentials, request IDs, private notes, catalog versions, capability keys and game secrets. Private request IDs are owner-only. A win alone does not create checkout, payment or an order.

Schema 6 captures each accepted bid in the same writer transaction, keyed by its
accepted row ID. Replay/rejection creates no extra interaction. Capture failure
rolls back bidding; unknown commit and lost-writer behavior remain fail-closed.
See `../interaction/README.md` and `../analytics/README.md`.

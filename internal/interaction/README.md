# First-party interactions

Schema 6 keeps a private allowlist: ID, kind, lot/event, nullable actor, original
occurrence/recording milliseconds, derived lot/event/actor demo flags, provenance,
and bounded source key. There is no metadata JSON, contact/address, request body,
cookie/token, IP, user agent, referrer, visitor identity or raw-event export.

`watchlist_add`, `bid_accepted`, `payment_confirmed` are inserted in the existing
leased writer transaction, after actual membership insertion, accepted bid and
first receipt respectively. Duplicate save/replayed bid/same-ID or fresh-ID
already-paid confirmation creates no event. Removal preserves history; re-add
creates a fresh transition. Capture failure rolls back the parent mutation.
Unknown commit is never reported as success; retry/restart reconciles durable
source keys. A lost leased writer is not reacquired by stale memory.

Migration backfills durable bid/receipt times as `backfilled`, and represents
current membership saved times as `snapshot`. It invents no removed memberships
or views. `observed` means captured since collection began, not verified human
traffic. Startup initializes collection coverage before readiness and never
moves it on restart. A completely unused migrated target remains importable
until startup. Explicit marked demo reset atomically replaces coverage/history.

## Anonymous view API

`POST /api/catalog/lot-views` accepts exactly:

```json
{"lot_id":"published-lot-id","observation_id":"00000000-0000-0000-0000-000000000001"}
```

Require `Content-Type: application/json`, `X-AuctionLab-Interaction: 1`, the
existing same-origin policy and <=1024 bytes. Only published lot/parent pairs
qualify. Lot IDs are bounded ASCII letters/digits, underscore or hyphen. A lowercase UUID binds globally to one lot. First/same-lot duplicate
confirmed commit returns 200 `{"data":{"recorded":true}}`. Rebinding is 409;
invalid shapes 422; malformed/unknown/duplicate fields 400; oversized 413;
boundary 403/415; draft/missing 404; storage/unknown commit 503. Responses are
no-store, issue no cookies, ignore account identity and log no collection payload.
The volatile direct-peer limiter allows 30 attempts/minute, retains <=4096 peer
keys, expires them and never trusts forwarded addresses.

A view is one received, rendered lot-detail document observation, not a card
impression, unique visitor or complete traffic history. The frontend uses an
in-memory UUID per lot/document, suppresses effect/language/execution refresh
repeats, and has no automatic retry or persistent tracking storage. Collection
failure is independent of catalog, recommendations and deliberate commerce.

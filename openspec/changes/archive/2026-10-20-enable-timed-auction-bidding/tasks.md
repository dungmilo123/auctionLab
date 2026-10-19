# Tasks

## 1. Additive auction storage

- [x] 1.1 Add migration `003_timed_auctions.sql` for per-lot runtime, accepted bids and terminal results with money/state/reference/uniqueness constraints and due/history/account indexes; verify fresh real PostgreSQL migration and invalid-row constraint checks.
- [x] 1.2 Embed migration 3 and advance supported schema/readiness version without editing migrations 1–2; verify fresh, repeated, failed-step, v2-upgrade and newer-schema-refusal checks preserve game/catalog/accounts/sessions/watchlists/capability data.
- [x] 1.3 Extend SQLite import occupancy checks for every new commerce auction table; verify occupied-target refusal and successful import/verify-only into an unused owned schema leave the source read-only and perform no auction execution.
- [x] 1.4 Update `docs/postgresql.md` for schema 3, stopped-writer upgrade and backup-based rollback; verify documented version/maintenance behavior matches migration and readiness checks and no schema-2 downgrade is implied.

## 2. Published schedule adoption and lifecycle

- [x] 2.1 Add the `internal/auction` domain/repository contracts and SQL implementation for repeat-safe published-only initialization from immutable catalog membership/schedules/pricing; verify drafts are excluded and existing future/current/expired publications retain their original dates.
- [x] 2.2 Implement scheduled/open/terminal transitions and per-lot revisions, including coherent opening/closing presentation states; verify injected-clock boundaries, overlapping events and independent staggered lots against real PostgreSQL.
- [x] 2.3 Persist once-only sold/unsold results and winner bid/account/amount references atomically; verify highest accepted bid wins, no-bid lots are unsold, repeated closure is harmless, and no order/payment/game mutation occurs.
- [x] 2.4 Wire pre-listener reconciliation and a cancellable bounded-batch worker in `cmd/server/main.go`; verify newly published lots activate without restart, overdue restart restores results once, maintenance modes start no worker, and shutdown joins worker before storage closes.
- [x] 2.5 Document activation of previously published schedules and preparation of new future-scheduled demo drafts in `README.md`; verify the rehearsal instructions do not edit/rebase an immutable published event or claim historical bids.

## 3. Transactional manual bid acceptance

- [x] 3.1 Implement cancellation-aware auction transactions on the existing leased writer connection with row locking and time/session revalidation after waiting; verify a pre-close arriving request is refused if validation occurs after close or session expiry.
- [x] 3.2 Implement exact USD minimum calculation and amount validation, including zero opening, arbitrary upward jumps, fixed increments, current-leader refusal and maximum-money exhaustion; verify deterministic domain tests and PostgreSQL acceptance/rejection leave consistent history/count/leader/revision.
- [x] 3.3 Add account-scoped request IDs and atomic accepted-receipt replay; verify concurrent identical submissions create one bid, changed payload conflicts, rejected IDs are unreserved, and replay works after restart/closure with a current session.
- [x] 3.4 Integrate auction commit-error classification and writer-loss handling; verify pre-commit failure rolls back all bid/result fields, uncertain commits do not report success, and lost ownership cannot reacquire or continue mutations.
- [x] 3.5 Verify bid-versus-bid and bid-versus-close races using real PostgreSQL and controlled time/ordering; assert unique sequences, one terminal result, exact winning cents and unchanged game revisions/publication.
- [x] 3.6 Record bid validation order, request replay and unknown-outcome recovery in `internal/auction/README.md`; verify documented examples match tested zero/opening/jump/late/replay behavior.

## 4. Public snapshots and private account APIs

- [x] 4.1 Add coherent published-only public auction snapshots and stable cursor history pages with per-lot Bidder N aliases; verify latest-50/history pagination, same-time sequence ordering, null pre-bid current price and stable aliases without public account identifiers.
- [x] 4.2 Extend the existing account API dispatch for `POST /api/account/bids`, `GET /api/account/bids` and private lot participation using its whole-namespace guard; verify strict JSON/size/header/origin/session rules, server-owned identity, HTTP status mapping and indistinguishable draft/missing targets.
- [x] 4.3 Add private participation projections and accepted-request reconciliation with one row per lot and deterministic ordering; verify two-account isolation, leading/outbid/closing/won/lost/not-participating states, and no rejected requests in history.
- [x] 4.4 Add wire/error privacy checks using distinctive account/catalog/game secret values; verify public snapshots/history/errors omit account IDs, emails, names, request IDs, private notes, capability keys and game secrets, and private reads expose only their owner.
- [x] 4.5 Document endpoint payloads, safe fields and status/retry semantics in `internal/auction/README.md`; verify examples against API tests and unchanged public catalog DTO/read contracts.

## 5. Realtime stream lifecycle

- [x] 5.1 Add read-only per-lot SSE with an initial full snapshot, revision/presentation updates, server-time heartbeats and a fresh full snapshot on reconnect; verify stream reads never mutate auctions and a commit racing a read appears on the next sample.
- [x] 5.2 Bound SSE connections/history/writes and cancel handlers before HTTP shutdown waits; verify capacity retry responses, stalled-reader termination, disconnect cleanup and shutdown with an active stream while bids/closure remain unblocked.
- [x] 5.3 Add typed frontend snapshot validation and per-lot revision merging; verify stale HTTP/SSE revisions cannot roll back state, same-revision time refresh works, and malformed data cannot activate submission.
- [x] 5.4 Implement stream reconnect/backoff, two-second fallback reads, ten-second staleness detection and focus/visibility cleanup; verify disconnect recovery, stale disabled controls, unmount cancellation and fresh server-time recovery with controlled timers.
- [x] 5.5 Update `web/README.md` with the commerce stream, existing `/api` proxy and recovery behavior; verify the documented stream works through Vite and embedded serving without opening `/ws` or adding external dependencies.

## 6. Lot-detail bidding experience

- [x] 6.1 Replace the disabled lot-detail panel with an independent execution loader, current price/minimum/count, accepted history/pagination and terminal result; verify catalog content remains usable on execution failure and condition/fulfillment still precede bid controls.
- [x] 6.2 Add exact decimal-to-cents manual submission with one request ID per pending intent, confirmed acceptance/refusal and explicit unknown-outcome reconciliation; verify no duplicate bid on retry, no optimistic acceptance and no automatic new-ID resubmission.
- [x] 6.3 Integrate current session-owned participation with public updates and two-second visible refresh; verify signed-out login return, unavailable identity, leading, stale, closing and terminal disabling, plus logout/account-change/obsolete-response clearing without bid replay.
- [x] 6.4 Add EN/VI labels, structured error copy, polite result announcements, keyboard focus and responsive history/form styling; verify focused state tests plus a keyboard review with no focus stealing or countdown announcement spam.
- [x] 6.5 Verify unchanged discovery URL phase semantics, opening-price sorts and catalog DTOs; update shared storefront/footer/admin availability copy and `web/README.md`, confirming accounts/watchlists/bidding are available while checkout/orders stay unavailable and watching/saving never bids.

## 7. Private bid participation page

- [x] 7.1 Replace the `/account/bids` preview with session-owned unique-lot participation, latest personal/current amounts, lot links and leading/outbid/won/lost filters with closing visible in All; verify filter/order/state rendering and won displays checkout unavailable.
- [x] 7.2 Add distinct loading, signed-out, empty, unavailable and retry states with visible polling/focus refresh; verify a failed read is not empty, account switching/logout clears prior bids, and delayed responses cannot leak stale private state.
- [x] 7.3 Verify direct load, refresh, safe login return, Back/Forward and native lot links in Vite and embedded builds; record observed routes and session isolation in the Change 5 verification evidence.
- [x] 7.4 Document the participation page and the boundary between winning and later checkout in `web/README.md`; verify interface copy in EN/VI and absence of invented orders, payment confirmations or other-account data.

## 8. Integrated rehearsal and acceptance evidence

- [x] 8.1 Run relevant backend suites and race checks with explicit `TEST_DATABASE_URL` and owned disposable schemas, then frontend tests/lint/build and the embedded no-CGO build; record exact commands/results and fail clearly if required PostgreSQL is unavailable.
- [x] 8.2 Rehearse two isolated bidder browsers competing on the same lot while other lots in overlapping events remain open; verify price/history/personal updates and a shared final winner, measure local bid/result visibility within two seconds, and save sanitized evidence under this change's `verification/`.
- [x] 8.3 Rehearse no-bid unsold, too-low/late/self-leading refusals, accepted retry after response loss, disconnect/reconnect and restart across a deadline; verify durable unique results, restored private participation and unchanged game/catalog/account/watchlist boundaries.
- [ ] 8.4 Verify lot-detail and `/account/bids` at 320/375 CSS pixels and desktop with keyboard navigation, EN/VI, reduced motion and actual 200-percent browser zoom; record screenshots/observations and leave any unavailable check uncompleted rather than substituting viewport emulation for zoom.
- [x] 8.5 Produce `verification/REPORT.md` mapping capability requirements to test/rehearsal evidence, changed-file scope and limitations; verify Change 4's archived tasks 7.3/8.3 and evidence are unchanged, no protected `AGENTS.md` edit occurred, and checkout/orders remain unavailable.

## Workflow follow-up

- Implementation starts only through a subsequent apply request; this checklist is not completion evidence.
- After implementation, request OpenSpec verification and archive separately, preserving any incomplete acceptance checks explicitly.

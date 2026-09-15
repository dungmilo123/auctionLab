/**
 * The LIVE bid controls' pure rules (D-71/D-72/D-73) — no DOM, no React, no
 * clock, so the frozen-value Vitest suite is the whole test surface (D-73).
 *
 * Two principles run through every function here:
 *
 *   - The SERVER is the only authority (D-52). `min_next_bid`, `budget`,
 *     `last_bid` and `is_top_bidder` all arrive on the wire; nothing in this
 *     module computes a price the ladder would have to re-derive.
 *   - The client's checks are the same checks the server makes — a below-minimum
 *     amount is refused here with the identical copy key the server's
 *     `bid.rejected` selects (UI-SPEC §LIVE, D-52/D-71).
 */

/** D-71's three quick steps, fixed by ENGL-01 — there is no configuration for them. */
export const QUICK_BID_DELTAS = [10_000, 50_000, 100_000] as const

/**
 * The amount a quick button sets: the server's next valid bid plus the delta —
 * never a step ladder the client invented (D-71).
 */
export function quickBidAmount(minNextBid: number, delta: number): number {
  return minNextBid + delta
}

/**
 * Parse the bid field's raw digit string (D-71: the input carries no grouping
 * dots — FRMT-01 in Phase 8 owns formatted inputs). Anything that is not a
 * plain run of digits is `null`: letters, `1.000`-style grouping, decimals,
 * signs, surrounding whitespace, scientific notation, and values beyond the
 * safe-integer range where the browser could no longer represent the amount
 * exactly (the server bounds every amount to 1e12).
 */
export function parseBidAmount(raw: string): number | null {
  if (!/^\d+$/.test(raw)) return null
  const amount = Number(raw)
  return Number.isSafeInteger(amount) ? amount : null
}

/**
 * The copy key one `bid.rejected.reason` selects (D-53). The reason vocabulary
 * is the server's payload-level set; every key below exists in the Phase-3
 * dictionary, and an unknown reason falls back to `error.generic` rather than
 * rendering the server's log-only `message` (UI-SPEC §Rejection feedback).
 */
export type BidReasonKey =
  | 'reason.invalidAmount'
  | 'reason.windowClosed'
  | 'reason.belowMinimum'
  | 'reason.exceedsBudget'
  | 'reason.alreadyLeading'
  | 'error.generic'

const REASON_KEYS: Readonly<Record<string, BidReasonKey>> = {
  invalid_amount: 'reason.invalidAmount',
  window_closed: 'reason.windowClosed',
  below_minimum: 'reason.belowMinimum',
  exceeds_budget: 'reason.exceedsBudget',
  already_leading: 'reason.alreadyLeading',
}

/** Map a rejection reason to its copy key; unknown reasons fall back. */
export function bidReasonKey(reason: string): BidReasonKey {
  return REASON_KEYS[reason] ?? 'error.generic'
}

/**
 * D-71's prefill rule: while the player has not edited the field, its value
 * follows the server's `min_next_bid` (entering LIVE, and every accepted bid);
 * once `dirty` is set — an `onChange` or a quick button — the hand-typed value
 * is never clobbered by a later minimum.
 */
export function bidInputValue(dirty: boolean, userValue: string, minNextBid: number): string {
  return dirty ? userValue : String(minNextBid)
}

/** What the D-72 banner needs to know about the current state. */
export type OutbidInput = {
  phase: string
  lastBid: number | null
  isTopBidder: boolean
  remainingMs: number
}

/**
 * D-72's banner, derived from state and nothing else: visible exactly when the
 * phase is LIVE, the player has bid in this run, the server says they are not
 * top, and the window is still open. It disappears by itself the moment they
 * are top again or the window closes — it is never a toast, a timer or a
 * notification.
 */
export function isOutbid(input: OutbidInput): boolean {
  return (
    input.phase === 'LIVE' &&
    input.lastBid !== null &&
    !input.isTopBidder &&
    input.remainingMs > 0
  )
}

/**
 * ECON-01: a minimum the budget cannot cover disables the field, the quick
 * buttons and the CTA. Equality is affordable — the server accepts a bid equal
 * to the budget — so only a strict shortfall disables anything.
 */
export function isOutOfBudget(budget: number, minNextBid: number): boolean {
  return budget < minNextBid
}

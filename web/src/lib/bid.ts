/**
 * The LIVE bid controls' pure rules (D-71/D-72/D-73) — RED stub.
 *
 * This commit lands the contract only: the amount arithmetic, the parse, the
 * reason→copy map, the prefill decision and the two state selectors are
 * implemented in the GREEN commit. No DOM, no React (D-73).
 */

/** D-71's three quick steps, fixed by ENGL-01 — there is no configuration for them. */
export const QUICK_BID_DELTAS = [10_000, 50_000, 100_000] as const

/** The amount a quick button sets. RED stub: no arithmetic yet. */
export function quickBidAmount(minNextBid: number, delta: number): number {
  void minNextBid
  void delta
  return 0
}

/** Parse the bid field's raw digit string. RED stub: no parse yet. */
export function parseBidAmount(raw: string): number | null {
  void raw
  return null
}

/**
 * The copy key one `bid.rejected.reason` selects (D-53). Every key exists in
 * the Phase-3 dictionary; an unknown reason falls back to `error.generic`.
 */
export type BidReasonKey =
  | 'reason.invalidAmount'
  | 'reason.windowClosed'
  | 'reason.belowMinimum'
  | 'reason.exceedsBudget'
  | 'reason.alreadyLeading'
  | 'error.generic'

/** Map a rejection reason to its copy key. RED stub: always the fallback. */
export function bidReasonKey(reason: string): BidReasonKey {
  void reason
  return 'error.generic'
}

/** D-71's prefill decision. RED stub: no decision yet. */
export function bidInputValue(dirty: boolean, userValue: string, minNextBid: number): string {
  void dirty
  void userValue
  void minNextBid
  return ''
}

/** What the D-72 banner needs to know about the current state. */
export type OutbidInput = {
  phase: string
  lastBid: number | null
  isTopBidder: boolean
  remainingMs: number
}

/** D-72's state-derived banner. RED stub: never visible yet. */
export function isOutbid(input: OutbidInput): boolean {
  void input
  return false
}

/** ECON-01's affordability check. RED stub: never disabled yet. */
export function isOutOfBudget(budget: number, minNextBid: number): boolean {
  void budget
  void minNextBid
  return false
}

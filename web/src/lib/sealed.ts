/**
 * The sealed amount field's pure rules (D-119/D-120) — no DOM, no React, no
 * clock, so the frozen-value Vitest suite is the whole test surface (the
 * `lib/bid.ts`/`lib/dutch.ts` convention, and the only way this phase can test
 * the form at all: `vitest` runs `environment: node`, so there is no DOM
 * harness).
 *
 * Two carried principles run through every function here, exactly as in
 * `lib/bid.ts`:
 *
 *   - The SERVER is the only authority (D-52). `start_price`, `my_bid` and
 *     `budget` all arrive on the wire; nothing here computes a price or decides
 *     an outcome.
 *   - The client's checks are the SAME checks the server makes (D-112): the
 *     bounds are inclusive on both ends, the message vocabulary is the shipped
 *     `reason.*` set, and the order is the engine's own
 *     (`invalid_amount` → `below_minimum` → `exceeds_budget`). The rules exist
 *     so the player has a field to fix, never to replace the refusal.
 */

import type { ConnectionStatus } from '@/ws/client'
import { parseBidAmount, type BidReasonKey } from './bid'

/** The amount field's own facts — what the message rule reads. */
export type SealedAmountInput = {
  /** the raw digit string as typed (D-119: never a formatted value) */
  raw: string
  /** the item's start price: the INCLUSIVE lower bound (SEAL-02) */
  startPrice: number
  /** the player's own budget: the INCLUSIVE upper bound (SEAL-02) */
  budget: number
}

/** Everything the enabled predicate reads: the field plus the window/socket. */
export type SealedSubmitInput = SealedAmountInput & {
  status: ConnectionStatus
  /** ms left on the locally anchored close clock (D-119) */
  remainingMs: number
  /** the carried in-flight rule: derived from the signal the frame was sent against */
  inFlight: boolean
}

/**
 * D-119's prefill rule, with `my_bid` in place of the English field's
 * `min_next_bid` (D-71's rule, same shape): while the player has not edited the
 * field, its value follows the amount the server says they already submitted —
 * so a reload mid-window re-renders exactly what they sent — and it is the empty
 * string while there is nothing to follow. Once `dirty` is set, the typed value
 * is never clobbered by a later frame, which is what lets a refused amount
 * survive for the player to fix (D-120).
 */
export function sealedInputValue(dirty: boolean, userValue: string, myBid: number | null): string {
  if (dirty) return userValue
  return myBid === null ? '' : String(myBid)
}

/**
 * The feedback slot's ONE sentence for the typed value, or null when the field
 * is usable. An EMPTY field is not an error (the carried nickname-field rule):
 * it is the initial state, and the disabled CTA is the whole feedback. The order
 * is the server's own (D-112), so a value that breaks two rules shows the
 * sentence the engine would answer with — including the engine's `amount <= 0`
 * first rung (`invalid_amount`), which a literal `0` hits before either bound
 * and even when `start_price` is `0`.
 */
export function sealedFieldError(input: SealedAmountInput): BidReasonKey | null {
  if (input.raw === '') return null
  const amount = parseBidAmount(input.raw)
  // `parseBidAmount` yields whole non-negative amounts, so `amount < 1` is the
  // engine's `amount <= 0` rung: a zero reads invalid_amount before the bounds
  // are consulted, exactly as `SubmitSealed` refuses it.
  if (amount === null || amount < 1) return 'reason.invalidAmount'
  if (amount < input.startPrice) return 'reason.belowMinimum'
  if (amount > input.budget) return 'reason.exceedsBudget'
  return null
}

/**
 * D-120's enabled predicate: the socket is `open` AND the window is open
 * (`remainingMs > 0`) AND nothing is in flight AND the typed value parses as a
 * positive amount AND `startPrice ≤ amount ≤ budget` — equality legal on both
 * bounds. It delegates to `sealedFieldError`, so the CTA and the sentence can
 * never disagree (the D-82b rule in player form): an enabled-but-refused submit
 * has nowhere to come from. The empty field is disabled here rather than refused
 * above, because an unfilled field is not an error. The server still decides
 * (D-52).
 */
export function canSubmitSealed(input: SealedSubmitInput): boolean {
  return (
    input.status === 'open' &&
    input.remainingMs > 0 &&
    !input.inFlight &&
    parseBidAmount(input.raw) !== null &&
    sealedFieldError(input) === null
  )
}

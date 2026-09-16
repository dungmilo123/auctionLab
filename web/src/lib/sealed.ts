/**
 * The sealed amount field's pure rules (D-119/D-120) — no DOM, no React, no
 * clock, so the frozen-value Vitest suite is the whole test surface (the
 * `lib/bid.ts` convention, and the only way this phase can test the form at all:
 * `vitest` runs `environment: node`, so there is no DOM harness).
 *
 * RED (06-03 Task 2): the rules below are DECLARED so `sealed.test.ts` loads and
 * fails on its assertions rather than on a missing module. Each body is the
 * trivial "not implemented" answer; GREEN replaces them with the pinned rules.
 */

import type { ConnectionStatus } from '@/ws/client'
import type { BidReasonKey } from './bid'

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

/** D-119's prefill: follow `my_bid` while the player has not edited the field. */
export function sealedInputValue(dirty: boolean, userValue: string, myBid: number | null): string {
  void myBid
  return userValue
}

/** The field's one sentence, or null when the typed value is usable. */
export function sealedFieldError(input: SealedAmountInput): BidReasonKey | null {
  void input
  return null
}

/** D-120's enabled predicate: one call the message and the CTA both read. */
export function canSubmitSealed(input: SealedSubmitInput): boolean {
  void input
  return false
}

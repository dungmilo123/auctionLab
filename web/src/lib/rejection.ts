/**
 * D-97's held-refusal rule, in a pure module: `(prev, snapshot) → prev | null`,
 * with no DOM, no React, no socket and no clock — the shape Vitest's node
 * environment pins exactly, and the convention `lib/bid.ts` and `lib/screen.ts`
 * already set ("everything here is a selector ... the only way to test the
 * route at all").
 *
 * A refusal is deliberately NOT view state (it is feedback, not a patch), so
 * the route decides when it goes away. D-97 narrows the carried "a rejection
 * clears on the next snapshot" rule for exactly one reason: the Dutch race
 * loser's `already_sold` is enqueued AFTER the winner's settle, so the reveal's
 * own snapshot has already landed by the time it arrives — and a later snapshot
 * for the same item (a join, a presence change, a resync during the reveal)
 * must not take the notice away. Carrying the item id is what makes the scope a
 * fact about the notice rather than a guess about frame order.
 *
 * `PlayerView` renders the held payload only while the item on screen is the
 * item it was refused for, so a stale notice cannot paint a later item's body
 * whatever this rule did.
 */

import type { BidRejected, PlayerSnapshot } from '@/ws/protocol'
import { bidReasonKey } from './bid'

/** D-53's refusal, held WITH the item it belongs to. */
export type HeldRejection = {
  payload: BidRejected
  /** the item the refusal belongs to; null before any item is current */
  itemId: string | null
}

/**
 * The next held refusal after one snapshot. Every reason keeps the carried
 * behaviour (any snapshot clears it); `already_sold` survives exactly while the
 * same item's SETTLED body is on screen and dies with a phase or item change.
 */
export function keepRejection(
  prev: HeldRejection | null,
  next: PlayerSnapshot,
): HeldRejection | null {
  if (prev === null) return null
  if (bidReasonKey(prev.payload.reason) !== 'reason.alreadySold') return null
  if (next.phase !== 'SETTLED' || next.item === null || next.item.id !== prev.itemId) return null
  return prev
}

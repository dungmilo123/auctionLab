/**
 * The client's Dutch price clock (D-91/D-93/D-94, DTCH-01) — pure, DOM-free and
 * clock-injectable, so the frozen-number Vitest suite in the node environment is
 * its whole test surface (D-73: no timer dependency, no DOM harness, no second
 * formula).
 *
 * It is the mirror image of `lib/countdown.ts` (D-60/D-61): a frame that carries
 * timing reduces to an anchor — the server's own ELAPSED time stands where the
 * countdown carries its remaining — and every later reading adds only LOCAL
 * elapsed time. The phone's absolute clock is never subtracted from a server
 * timestamp.
 *
 * A Dutch price is a DISPLAY, never an authority: the server derives and charges
 * its own number (D-91), so this module only ever fills the gap between two
 * frames. At the anchor instant it answers exactly the `current_price` the frame
 * carried, which is the honesty property the suite pins.
 *
 * Both bodies that render a falling price — the player's LIVE body and the
 * projector's live arm — call this one module (D-103/D-105), so the phone and
 * the room can never disagree about what the price is doing.
 */

import type { ConnectionStatus } from '@/ws/client'

/**
 * One Dutch timing anchor: how long the server's price clock had already run
 * when the frame was built, plus the local instant that frame arrived. The
 * server's own ELAPSED time stands where `countdown.ts` carries its remaining.
 */
export type DutchAnchor = {
  /** ms the server's price clock had run when this frame was built */
  elapsedAtAnchor: number
  /** the local clock reading when that frame arrived */
  localAtAnchor: number
}

/** The item's own Dutch mechanics (D-91): the numbers a price derives from. */
export type DutchMechanics = {
  startPrice: number
  floorPrice: number
  decrementStep: number
  decrementIntervalMs: number
}

/** What D-104's ACCEPT CTA needs in order to enable itself. */
export type AcceptInput = {
  /** the socket's own status — a control that cannot deliver must not look available */
  status: ConnectionStatus
  /** true while an accept is in flight */
  inFlight: boolean
  /** the price derived for this render — the same number the price well shows */
  price: number
  /** the player's own budget, from the server's snapshot */
  budget: number
}

/**
 * Anchor on one frame that carried timing. `startedAt` and `serverTs` are the
 * server's numbers (Unix ms, D-94) and `localNow` is the phone's own reading at
 * the frame's arrival — the only local value the maths ever uses (D-60's rule).
 * A frame whose anchor lies in the future (a skewed server clock) anchors at
 * zero elapsed rather than at a negative one, because a price above the start
 * could never be charged.
 */
export function anchorDutch(startedAt: number, serverTs: number, localNow: number): DutchAnchor {
  return { elapsedAtAnchor: Math.max(0, serverTs - startedAt), localAtAnchor: localNow }
}

/**
 * The server elapsed the anchor carries plus the LOCAL elapsed since it was
 * taken — the only clock this module reads, and never a negative number, so a
 * stale anchor cannot produce a price that runs backwards.
 */
export function elapsedOf(a: DutchAnchor, nowMs: number): number {
  return Math.max(0, a.elapsedAtAnchor + (nowMs - a.localAtAnchor))
}

/**
 * D-91's ONE client derivation: `max(start − floor(elapsed / interval) × step,
 * floor)`, held inside the item's own `[floor, start]` range.
 *
 * Total by construction, because a projector must never freeze and no body may
 * print NaN:
 *
 *   - a step or interval that is not a positive number — NaN included, which is
 *     why the test is written the positive way round — has no clock, and the
 *     start price is the only honest answer (D-92 refuses to open such a run);
 *   - a negative elapsed clamps to zero, so the derivation can never report a
 *     price above the start (D-91);
 *   - the step count is clamped in the DIVISION domain exactly as the server
 *     clamps it (research Pitfall 4): past the floor the price is pinned anyway,
 *     so a larger count is indistinguishable, while an unclamped product would
 *     be a plausible-looking but wrong number between the floor and the start.
 *
 * The floor clamp is the server's own, and the start clamp is applied first so
 * that a row the live-start guard refuses (a floor above the start) still gets
 * the server's answer rather than a second client-side rule.
 */
export function dutchPrice(mechanics: DutchMechanics, elapsedMs: number): number {
  const { startPrice, floorPrice, decrementStep, decrementIntervalMs } = mechanics
  if (!(decrementStep >= 1) || !(decrementIntervalMs >= 1)) return startPrice

  const elapsed = Math.max(0, elapsedMs)
  let steps = Math.floor(elapsed / decrementIntervalMs)

  const span = startPrice - floorPrice
  if (span >= 0) {
    steps = Math.min(steps, Math.floor(span / decrementStep) + 1)
  } else {
    steps = 0
  }

  return Math.max(floorPrice, Math.min(startPrice - steps * decrementStep, startPrice))
}

/**
 * True exactly while the price is pinned at the floor (D-103's at-floor line):
 * the number has stopped moving, so the body says so instead of implying it can
 * still fall.
 */
export function atFloor(price: number, floor: number): boolean {
  return price <= floor
}

/**
 * D-104's one ACCEPT predicate, re-derived on every render so the CTA re-enables
 * by itself as the price falls past the budget — never a snapshot-only decision.
 * Equality IS affordable: the server charges a price equal to the budget
 * (`price <= budget`), so only a strict shortfall disables the button. The socket
 * must be open and nothing may be in flight, because a control that cannot
 * deliver must not look available (D-82a's rule, applied to the accept).
 */
export function canAccept(input: AcceptInput): boolean {
  return input.status === 'open' && !input.inFlight && input.price <= input.budget
}

/**
 * The client's Dutch price clock (D-91/D-93/D-94, DTCH-01) — RED DECLARATIONS.
 *
 * Every export below is a sentinel: the types, the names and the doc contracts
 * are final, the bodies are not. `dutch.test.ts` is the failing suite that pins
 * the behaviour and the GREEN commit replaces each body with the ONE client
 * derivation — 04-04's/04-06's/04-07's declared-sentinel pattern, so the suite
 * COMPILES and fails on assertions instead of on a missing identifier (#3770).
 *
 * The module is the mirror image of `lib/countdown.ts` (D-60/D-61): pure, DOM-free
 * and clock-injectable, so the frozen-number Vitest suite in the node
 * environment is its whole test surface (D-73 — no fake timers, no DOM harness,
 * no second formula). It derives the falling number from the item's own
 * mechanics and an anchor of `(started_at, serverTs, localNow)`, and only LOCAL
 * elapsed time is ever added afterwards: the phone's absolute clock is never
 * subtracted from a server timestamp.
 *
 * A Dutch price is a DISPLAY, never an authority: the server's own number is the
 * price that is charged (D-91), so this module only ever fills the gap between
 * two frames — at the anchor instant it answers exactly the `current_price` the
 * frame carried.
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
export function anchorDutch(_startedAt: number, _serverTs: number, localNow: number): DutchAnchor {
  return { elapsedAtAnchor: 0, localAtAnchor: localNow }
}

/**
 * The server elapsed the anchor carries plus the LOCAL elapsed since it was
 * taken — the only clock this module reads, and never a negative number.
 */
export function elapsedOf(_a: DutchAnchor, _nowMs: number): number {
  return 0
}

/**
 * D-91's ONE client derivation: `max(start − floor(elapsed / interval) × step,
 * floor)`, clamped into the item's own range. Total by construction — a row
 * without a usable clock answers the start price, a negative elapsed clamps to
 * zero, and the step count is clamped in the division domain so a huge elapsed
 * can never wrap.
 */
export function dutchPrice(_mechanics: DutchMechanics, _elapsedMs: number): number {
  return 0
}

/** True exactly while the price is pinned at the floor (D-103's at-floor line). */
export function atFloor(_price: number, _floor: number): boolean {
  return false
}

/**
 * D-104's one ACCEPT predicate, re-derived on every render: the socket must be
 * open, no accept may be in flight, and the price must be affordable — equality
 * IS affordable, because the server charges a price equal to the budget.
 */
export function canAccept(_input: AcceptInput): boolean {
  return false
}

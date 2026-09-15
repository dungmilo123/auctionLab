/**
 * The client's countdown anchor (D-60/D-61, TIME-02) — RED stub.
 *
 * This commit lands the contract only: the anchor arithmetic and the mm:ss
 * formatting are implemented in the GREEN commit. Nothing here reads a clock,
 * which is what lets the frozen-time suite drive it (D-73).
 */

/** One timing anchor: what was left, and the local instant it was reported at. */
export type TimingAnchor = {
  /** ms left on the server clock when the frame was built */
  remainingAtAnchor: number
  /** the local clock reading when that frame arrived */
  localAtAnchor: number
}

/** Anchor on one frame that carried timing. RED stub: no maths yet. */
export function anchor(deadlineAt: number, serverTs: number, localNow: number): TimingAnchor {
  void deadlineAt
  void serverTs
  return { remainingAtAnchor: 0, localAtAnchor: localNow }
}

/** ms left at local time `nowMs`. RED stub: no maths yet. */
export function remainingMs(a: TimingAnchor, nowMs: number): number {
  void a
  void nowMs
  return 0
}

/** mm:ss. RED stub: no formatting yet. */
export function formatClock(ms: number): string {
  void ms
  return ''
}

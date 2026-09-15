/**
 * The client's countdown anchor (D-60/D-61, TIME-02) — pure, DOM-free and
 * clock-injectable, so the frozen-time Vitest suite can drive it (D-73: no
 * timer dependency, no DOM harness).
 *
 * The phone's absolute clock is NEVER subtracted from a server timestamp. A
 * frame that carries timing — a `state.snapshot` (`server_ts` + `item.deadline_at`)
 * or a `price.tick` (envelope `ts` + `deadline_at`) — is reduced to an anchor:
 * the remaining duration the server reported, plus the local instant that frame
 * arrived. Every later reading is that remaining duration minus the LOCAL
 * elapsed time, so a phone whose clock is three minutes off still shows the
 * right countdown (D-60).
 *
 * Reaching zero here only stops the display: the client never invents an
 * auction end state, and the settle always arrives from the server
 * (UI-SPEC §Countdown).
 */

/** One timing anchor: what was left, and the local instant it was reported at. */
export type TimingAnchor = {
  /** ms left on the server clock when the frame was built */
  remainingAtAnchor: number
  /** the local clock reading when that frame arrived */
  localAtAnchor: number
}

/**
 * Anchor on one frame that carried timing. `deadlineAt` and `serverTs` are the
 * server's numbers (Unix ms, D-60) and `localNow` is the phone's own reading at
 * the frame's arrival — the only local value the maths ever uses. A frame whose
 * deadline has already passed anchors at zero rather than at a negative
 * remaining, because the countdown that follows is a display, not an authority.
 */
export function anchor(deadlineAt: number, serverTs: number, localNow: number): TimingAnchor {
  return { remainingAtAnchor: Math.max(0, deadlineAt - serverTs), localAtAnchor: localNow }
}

/**
 * ms left at local time `nowMs` — the elapsed local time is the only thing
 * subtracted, and the result is clamped at zero so a stale anchor can never
 * produce a negative clock.
 */
export function remainingMs(a: TimingAnchor, nowMs: number): number {
  return Math.max(0, a.remainingAtAnchor - (nowMs - a.localAtAnchor))
}

/**
 * mm:ss: floored to whole seconds, zero-padded, minutes uncapped, `00:00` at
 * zero. A negative remainder renders as `00:00` rather than as a negative
 * clock. The UI re-renders this roughly every 250 ms from its own component —
 * this function itself has no interval (UI-SPEC §Countdown).
 */
export function formatClock(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`
}

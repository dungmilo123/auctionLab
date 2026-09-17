/**
 * The reconnect policy (D-130…D-134) — pure, DOM-free and socket-free, so
 * Vitest's `node` environment can pin every threshold without a browser harness
 * and without a timer.
 *
 * `client.ts` owns the globals this policy is applied through (`window`,
 * `document`, `WebSocket`, `localStorage`, `setTimeout`, `Math.random`). This
 * module owns the RULES those globals apply, so a threshold can only be changed
 * here and only with a test:
 *
 *   - D-130's full-jitter backoff: uniform in [0, min(500 ms · 2^n, 15 s)];
 *   - the four derived states and the two thresholds that separate them;
 *   - which word the indicator renders, including G-1's pre-first-open case;
 *   - D-131's terminal 4001 decision — stop the scheduler, keep the token;
 *   - D-132's liveness budget (the caller's staleness interval reads it);
 *   - D-133's hidden-tab threshold (a longer hide replaces the socket);
 *   - D-134's 8-second bounded restore and its closed clearing vocabulary.
 *
 * Nothing here reads a clock, a random source or the DOM: `now`, `rand` and
 * `hiddenForMs` are all parameters (the `gate.ts` / `dutch.ts` precedent). The
 * numbers are exported so no caller can re-spell one of them, and every one of
 * them is pinned by `reconnect.test.ts`.
 *
 * The `Math.*` calls below are pure arithmetic over their arguments — this
 * module never calls `Math.random` itself, and a jitter that is not a secret
 * must never reach for `crypto` (the `math/rand/v2` half of the repo's random
 * rule; the `crypto/rand` half is the token path in `client.ts`).
 */

/** D-130's base delay: the first retry waits this long at most. */
export const BACKOFF_BASE_MS = 500

/** D-130's cap: no automatic attempt ever waits longer than this. */
export const BACKOFF_CAP_MS = 15_000

/** D-130: this many consecutive failed attempts is `offline`. */
export const OFFLINE_ATTEMPT_THRESHOLD = 6

/** D-130: this long since the last successful open is also `offline`. */
export const OFFLINE_AGE_MS = 30_000

/** D-132: silence longer than this force-closes the socket. */
export const STALE_SILENCE_MS = 60_000

/** D-132: how often the visible-only staleness check runs. */
export const STALE_CHECK_PERIOD_MS = 5_000

/** D-133: a tab hidden longer than this gets a NEW socket on return. */
export const HIDDEN_TEARDOWN_MS = 5_000

/** D-134: the restoring line's bound, measured from the route's mount. */
export const RESTORE_BOUND_MS = 8_000

/** 4001: another connection took this identity (D-31). Terminal per page. */
export const SUPERSEDED_CLOSE_CODE = 4001

/** The four connection states UI-SPEC allows — exactly these, no more. */
export type ReconnectState = 'live' | 'reconnecting' | 'offline' | 'superseded'

/** One delay from D-130's full-jitter policy: uniform in [0, min(500·2^n, 15 s)]. */
export function nextDelay(attempt: number, rand: number): number {
  return BACKOFF_BASE_MS
}

/** Everything the derived state is a function of. */
export type ReconnectInputs = {
  /** true while the transport is OPEN (readyState 1) */
  open: boolean
  /** consecutive failed attempts since the last successful open */
  attempts: number
  /** this page instance's last successful open, or null if it never opened */
  lastOpenAt: number | null
  /** the caller's clock, in milliseconds */
  now: number
  /** the code that ended the last connection, or null if there was none */
  lastCloseCode: number | null
}

/** The state the indicator declares for these inputs (D-130). */
export function deriveState(inputs: ReconnectInputs): ReconnectState {
  return 'reconnecting'
}

/** The dictionary keys the indicator can render — one per state cell (G-1). */
export type ConnectionWord =
  | 'status.connecting'
  | 'status.open'
  | 'status.reconnecting'
  | 'status.lost'
  | 'status.superseded'

/** The word for one state, with `reconnecting` split by G-1's first-open rule. */
export function connectionWord(state: ReconnectState, lastOpenAt: number | null): ConnectionWord {
  return 'status.connecting'
}

/** What the scheduler must do next (D-130's "never stops" vs D-131's terminal). */
export type SchedulerDecision = 'retry' | 'stop'

/** D-131: `superseded` stops the automatic scheduler for this page instance. */
export function schedulerDecision(state: ReconnectState): SchedulerDecision {
  return 'retry'
}

/** D-134: the signals that end the restoring line before its bound. */
export type RestoreSignal = 'close' | 'snapshot' | 'session_ended' | 'token_invalid' | 'error'

/** The closed vocabulary of D-134's clearing events — exactly these five. */
export const RESTORE_CLEARING_SIGNALS: readonly RestoreSignal[] = [
  'close',
  'snapshot',
  'session_ended',
  'token_invalid',
  'error',
]

/** D-134's bounded restoring line: true while the mount is young and nothing cleared it. */
export function restoreActive(inputs: {
  mountedAt: number
  now: number
  clearedBy: RestoreSignal | null
}): boolean {
  return true
}

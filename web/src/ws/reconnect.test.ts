/**
 * The reconnect policy's rules (D-130…D-134).
 *
 * Frozen numbers only, exactly as `dutch.test.ts` and `gate.test.ts` pin their
 * derivations: the module never reads a clock, never calls `Math.random` and
 * never touches the DOM, so every case below is a pure input→output pair. No
 * jsdom, no timers, no `vi.stubGlobal`.
 */

import { describe, expect, it } from 'vitest'
import {
  BACKOFF_BASE_MS,
  BACKOFF_CAP_MS,
  HIDDEN_TEARDOWN_MS,
  OFFLINE_AGE_MS,
  OFFLINE_ATTEMPT_THRESHOLD,
  RESTORE_BOUND_MS,
  RESTORE_CLEARING_SIGNALS,
  STALE_CHECK_PERIOD_MS,
  STALE_SILENCE_MS,
  SUPERSEDED_CLOSE_CODE,
  connectionWord,
  deriveState,
  nextDelay,
  restoreActive,
  schedulerDecision,
  type ReconnectInputs,
  type RestoreSignal,
} from './reconnect'

/** One input set: an open socket is the default, so each case names only what it changes. */
function at(overrides: Partial<ReconnectInputs>): ReconnectInputs {
  return { open: true, attempts: 0, lastOpenAt: null, now: 0, lastCloseCode: null, ...overrides }
}

describe('the full-jitter backoff (D-130)', () => {
  it('pins the policy numbers', () => {
    expect(BACKOFF_BASE_MS).toBe(500)
    expect(BACKOFF_CAP_MS).toBe(15_000)
    expect(OFFLINE_ATTEMPT_THRESHOLD).toBe(6)
    expect(OFFLINE_AGE_MS).toBe(30_000)
    expect(STALE_SILENCE_MS).toBe(60_000)
    expect(HIDDEN_TEARDOWN_MS).toBe(5_000)
    expect(RESTORE_BOUND_MS).toBe(8_000)
    expect(SUPERSEDED_CLOSE_CODE).toBe(4001)
  })

  it('spreads attempt n over [0, min(500·2^n, 15 000)]', () => {
    // The endpoints of the interval are the policy: no rand value can escape it.
    expect(nextDelay(0, 0)).toBe(0)
    expect(nextDelay(0, 1)).toBe(500)
    expect(nextDelay(1, 0.5)).toBe(500)
    expect(nextDelay(2, 0.5)).toBe(1000)
    expect(nextDelay(4, 0.5)).toBe(4000)
    expect(nextDelay(5, 0.5)).toBe(7500)
    // Attempt 5 is where 500·2^5 (16 000) first exceeds the cap, so the whole
    // interval above it is capped — including attempts that overflow a double.
    expect(nextDelay(5, 1)).toBe(15_000)
    expect(nextDelay(6, 1)).toBe(15_000)
    expect(nextDelay(40, 1)).toBe(15_000)
  })

  it('answers inside the bound for every attempt and rand it can be given', () => {
    for (let attempt = 0; attempt <= 12; attempt += 1) {
      const ceiling = Math.min(BACKOFF_BASE_MS * 2 ** attempt, BACKOFF_CAP_MS)
      for (const rand of [0, 0.25, 0.5, 0.75, 0.999_999]) {
        const delay = nextDelay(attempt, rand)
        expect(delay).toBeGreaterThanOrEqual(0)
        expect(delay).toBeLessThanOrEqual(ceiling)
      }
    }
  })

  it('is total: an unusable attempt falls back to the base rather than to NaN', () => {
    // A count is never negative and never fractional; the answer stays a delay
    // instead of a NaN that `setTimeout` would silently turn into 0 ms (the
    // `dutchPriceAt` total-answer rule).
    expect(nextDelay(-3, 1)).toBe(500)
    expect(nextDelay(1.7, 1)).toBe(1000)
    expect(nextDelay(0, -1)).toBe(0)
    expect(nextDelay(0, 2)).toBe(500)
  })
})

describe('the derived state (D-130)', () => {
  it('is live while the socket is open', () => {
    expect(deriveState(at({ open: true }))).toBe('live')
  })

  it('is reconnecting before the first open and while a retry is scheduled', () => {
    expect(deriveState(at({ open: false }))).toBe('reconnecting')
    expect(deriveState(at({ open: false, attempts: 1, lastOpenAt: 0, now: 1 }))).toBe('reconnecting')
    // One short of each threshold: still reconnecting.
    expect(deriveState(at({ open: false, attempts: 5 }))).toBe('reconnecting')
    expect(deriveState(at({ open: false, lastOpenAt: 1_000, now: 1_000 + OFFLINE_AGE_MS - 1 }))).toBe(
      'reconnecting',
    )
  })

  it('turns offline at six failed attempts', () => {
    expect(deriveState(at({ open: false, attempts: 6 }))).toBe('offline')
    expect(deriveState(at({ open: false, attempts: 40 }))).toBe('offline')
  })

  it('turns offline thirty seconds after the last successful open', () => {
    expect(deriveState(at({ open: false, lastOpenAt: 0, now: OFFLINE_AGE_MS }))).toBe('offline')
    // A page that never opened is not aged out by a clock it never had.
    expect(deriveState(at({ open: false, lastOpenAt: null, now: 10_000_000 }))).toBe('reconnecting')
  })

  it('is superseded on a 4001 close, terminally', () => {
    expect(deriveState(at({ open: false, lastCloseCode: 4001 }))).toBe('superseded')
    // Terminal wins over every other input: no later reading of the same close
    // can make the page try again (D-131).
    expect(deriveState(at({ open: true, lastCloseCode: 4001 }))).toBe('superseded')
    // 4002 is NOT this state — the carried session-end behaviour is unchanged.
    expect(deriveState(at({ open: false, lastCloseCode: 4002 }))).toBe('reconnecting')
  })
})

describe('the indicator word (G-1)', () => {
  it('renders one word per state, and splits reconnecting by the first open', () => {
    expect(connectionWord('live', null)).toBe('status.open')
    expect(connectionWord('offline', 1_000)).toBe('status.lost')
    expect(connectionWord('superseded', null)).toBe('status.superseded')
    // Before the first successful open, "Reconnecting…" would be a lie.
    expect(connectionWord('reconnecting', null)).toBe('status.connecting')
    expect(connectionWord('reconnecting', 1_000)).toBe('status.reconnecting')
  })
})

describe('the scheduler decision (D-130 G-2 / D-131)', () => {
  it('keeps retrying in every state except the terminal one', () => {
    // AUTO-RETRY NEVER STOPS: `offline` is a presentation threshold, not a stop.
    expect(schedulerDecision('live')).toBe('retry')
    expect(schedulerDecision('reconnecting')).toBe('retry')
    expect(schedulerDecision('offline')).toBe('retry')
    expect(schedulerDecision('superseded')).toBe('stop')
  })
})

describe('the bounded restore (D-134)', () => {
  it('renders while the mount is inside the bound and nothing has cleared it', () => {
    expect(restoreActive({ mountedAt: 0, now: 0, clearedBy: null })).toBe(true)
    expect(restoreActive({ mountedAt: 0, now: RESTORE_BOUND_MS - 1, clearedBy: null })).toBe(true)
  })

  it('stops at the bound, exactly', () => {
    expect(restoreActive({ mountedAt: 0, now: RESTORE_BOUND_MS, clearedBy: null })).toBe(false)
    expect(restoreActive({ mountedAt: 0, now: RESTORE_BOUND_MS + 1, clearedBy: null })).toBe(false)
  })

  it('clears early on every signal in the closed vocabulary', () => {
    expect([...RESTORE_CLEARING_SIGNALS]).toEqual([
      'close',
      'snapshot',
      'session_ended',
      'token_invalid',
      'error',
    ])
    for (const signal of RESTORE_CLEARING_SIGNALS) {
      expect(restoreActive({ mountedAt: 0, now: 0, clearedBy: signal })).toBe(false)
    }
    // The parameter's own type is the closed set: a signal outside it is a
    // compile error, not a silent extra clear.
    const exhaustive: RestoreSignal[] = ['close', 'snapshot', 'session_ended', 'token_invalid', 'error']
    expect(exhaustive).toEqual([...RESTORE_CLEARING_SIGNALS])
  })
})

describe('the staleness budget (D-132)', () => {
  it('is longer than the server beat by more than a factor of two', () => {
    // The server emits one heartbeat every 25 s; the budget must tolerate a
    // missed beat without ever accepting a silently dead socket.
    expect(STALE_SILENCE_MS).toBeGreaterThan(25_000 * 2)
    expect(STALE_CHECK_PERIOD_MS).toBeGreaterThan(0)
    expect(STALE_CHECK_PERIOD_MS).toBeLessThanOrEqual(STALE_SILENCE_MS)
  })
})

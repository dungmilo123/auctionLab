import { describe, expect, it } from 'vitest'
import { anchor, formatClock, remainingMs } from './countdown'

// Frozen numbers only (D-61): the module never reads a clock and never touches
// the DOM, so every case is a pure input→output pair a frozen-time suite can
// pin (D-73 — no timer dependency, no DOM harness).
const SERVER_TS = 1_757_934_000_000
const LOCAL_NOW = 4_096

describe('anchor (D-60)', () => {
  it('turns a server deadline plus the server clock into a local anchor', () => {
    expect(anchor(SERVER_TS + 30_000, SERVER_TS, LOCAL_NOW)).toEqual({
      remainingAtAnchor: 30_000,
      localAtAnchor: LOCAL_NOW,
    })
  })

  it('clamps an already-elapsed deadline to zero remaining', () => {
    expect(anchor(SERVER_TS - 1, SERVER_TS, LOCAL_NOW).remainingAtAnchor).toBe(0)
  })

  it('never copies a server absolute value into the local anchor', () => {
    // The anchor carries the server's DURATION and the phone's own reading at
    // arrival — the phone's absolute clock is never compared to a server stamp.
    const a = anchor(SERVER_TS + 60_000, SERVER_TS, LOCAL_NOW)
    expect(a.localAtAnchor).toBe(LOCAL_NOW)
    expect(a.localAtAnchor).not.toBe(SERVER_TS)
  })
})

describe('remainingMs (D-61)', () => {
  it('subtracts only LOCAL elapsed time from the server anchor', () => {
    const a = anchor(SERVER_TS + 60_000, SERVER_TS, LOCAL_NOW)
    expect(remainingMs(a, LOCAL_NOW)).toBe(60_000)
    expect(remainingMs(a, LOCAL_NOW + 250)).toBe(59_750)
    expect(remainingMs(a, LOCAL_NOW + 59_000)).toBe(1_000)
  })

  it('clamps at zero once the elapsed time passes the anchor', () => {
    const a = anchor(SERVER_TS + 5_000, SERVER_TS, LOCAL_NOW)
    expect(remainingMs(a, LOCAL_NOW + 5_000)).toBe(0)
    expect(remainingMs(a, LOCAL_NOW + 60_000)).toBe(0)
  })

  it('re-anchors on every frame that carries timing', () => {
    // Two frames five seconds apart, each carrying its own (deadline, ts) pair:
    // the newest anchor wins outright and the countdown never double-counts the
    // earlier one (UI-SPEC §Countdown).
    const first = anchor(SERVER_TS + 30_000, SERVER_TS, LOCAL_NOW)
    const second = anchor(SERVER_TS + 25_000, SERVER_TS + 5_000, LOCAL_NOW + 5_000)
    expect(remainingMs(first, LOCAL_NOW + 5_000)).toBe(25_000)
    expect(remainingMs(second, LOCAL_NOW + 5_000)).toBe(25_000)
    expect(remainingMs(second, LOCAL_NOW + 4_900)).toBe(24_900)
  })

  it('is unaffected by a phone clock that is minutes off', () => {
    // The local stamps cancel out: a clock three minutes fast changes both
    // endpoints of the difference, never the answer (D-60's whole point).
    const skewed = anchor(SERVER_TS + 30_000, SERVER_TS, LOCAL_NOW + 180_000)
    expect(remainingMs(skewed, LOCAL_NOW + 180_000 + 10_000)).toBe(20_000)
  })
})

describe('formatClock (D-61)', () => {
  const cases: Array<[ms: number, expected: string]> = [
    [0, '00:00'],
    [999, '00:00'], // floored to whole seconds
    [1_000, '00:01'],
    [9_000, '00:09'], // zero-padded seconds
    [60_000, '01:00'],
    [61_000, '01:01'],
    [3_599_999, '59:59'],
    [3_660_000, '61:00'], // minutes are uncapped
  ]

  it.each(cases)('renders %d ms as %s', (ms, expected) => {
    expect(formatClock(ms)).toBe(expected)
  })

  it('renders a negative remainder as 00:00 rather than a negative clock', () => {
    expect(formatClock(-1)).toBe('00:00')
  })
})

import { describe, expect, it } from 'vitest'
import { anchorDutch, atFloor, canAccept, dutchPrice, elapsedOf, type DutchMechanics } from './dutch'

// Frozen numbers only (D-61/D-73): the module never reads a clock and never
// touches the DOM, so every case is a pure input→output pair this suite can pin
// without a timer dependency and without a DOM harness.
const SERVER_TS = 1_757_934_000_000
const LOCAL_NOW = 4_096
/** The instant the server's Dutch clock started, four seconds before the frame. */
const STARTED_AT = SERVER_TS - 4_000

/** The fixture item (D-91): 300.000 falling 20.000 every second to a 100.000 floor. */
const MECHANICS: DutchMechanics = {
  startPrice: 300_000,
  floorPrice: 100_000,
  decrementStep: 20_000,
  decrementIntervalMs: 1_000,
}

/** What the server itself counts at the anchor instant: 300.000 − 4 × 20.000. */
const FRAME_PRICE = 220_000

describe('anchorDutch (D-60/D-94)', () => {
  it('turns a start instant and the server clock into the server elapsed plus the local instant', () => {
    expect(anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW)).toEqual({
      elapsedAtAnchor: 4_000,
      localAtAnchor: LOCAL_NOW,
    })
  })

  it('clamps an anchor that lies in the future to zero elapsed', () => {
    // A skewed server clock never buys a price above the start (D-91).
    expect(anchorDutch(STARTED_AT + 5_000, STARTED_AT, LOCAL_NOW).elapsedAtAnchor).toBe(0)
  })

  it('never copies a server absolute value into the local anchor', () => {
    // The anchor carries the server's ELAPSED time and the phone's own reading
    // at arrival — the phone's absolute clock is never compared to a server
    // stamp (D-60's whole point, mirrored from countdown.ts).
    const a = anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW)
    expect(a.localAtAnchor).toBe(LOCAL_NOW)
    expect(a.localAtAnchor).not.toBe(SERVER_TS)
  })
})

describe('elapsedOf (D-94)', () => {
  it('adds only LOCAL elapsed time to the server elapsed it anchored on', () => {
    const a = anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW)
    expect(elapsedOf(a, LOCAL_NOW)).toBe(4_000)
    expect(elapsedOf(a, LOCAL_NOW + 250)).toBe(4_250)
    expect(elapsedOf(a, LOCAL_NOW + 11_000)).toBe(15_000)
  })

  it('is unaffected by a phone clock that is minutes off', () => {
    // Both endpoints of the local difference move together, so the answer does
    // not depend on the phone's absolute clock.
    const skewed = anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW + 180_000)
    expect(elapsedOf(skewed, LOCAL_NOW + 180_000 + 10_000)).toBe(14_000)
  })

  it('never reports a negative elapsed', () => {
    const a = anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW)
    expect(elapsedOf(a, LOCAL_NOW - 60_000)).toBe(0)
  })

  it('re-anchors on every frame that carries timing — the later frame stands alone', () => {
    // Two frames five seconds apart carry the same started_at with a newer
    // server clock: each anchor is used on its own, so the price they describe
    // at the same instant is the same number.
    const first = anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW)
    const second = anchorDutch(STARTED_AT, SERVER_TS + 5_000, LOCAL_NOW + 5_000)
    expect(elapsedOf(first, LOCAL_NOW + 5_000)).toBe(9_000)
    expect(elapsedOf(second, LOCAL_NOW + 5_000)).toBe(9_000)
    expect(elapsedOf(second, LOCAL_NOW + 5_900)).toBe(9_900)
  })

  it('keeps the held anchor when a frame carries no start instant — the price never blanks', () => {
    // A tick without started_at is not a re-anchor (05-UI-SPEC §partial): the
    // body carries on interpolating from the anchor it already holds, so the
    // number keeps falling on the same path — it neither blanks nor jumps back
    // to the start price.
    const held = anchorDutch(STARTED_AT, SERVER_TS, LOCAL_NOW)
    const atAnchor = dutchPrice(MECHANICS, elapsedOf(held, LOCAL_NOW))
    const later = dutchPrice(MECHANICS, elapsedOf(held, LOCAL_NOW + 3_000))
    expect(atAnchor).toBe(FRAME_PRICE)
    expect(later).toBe(160_000)
    expect(later).toBeLessThan(atAnchor)
  })
})

describe('dutchPrice (D-91)', () => {
  const cases: Array<[elapsed: number, expected: number, label: string]> = [
    [0, 300_000, 'the start price at zero elapsed'],
    [999, 300_000, 'still the start price one ms before the first step'],
    [1_000, 280_000, 'exactly one step at k × interval'],
    [1_999, 280_000, 'no second step one ms before it'],
    [2_000, 260_000, 'two steps at 2 × interval'],
    [4_000, FRAME_PRICE, "the frame's own current_price at the anchor instant"],
    [9_999, 120_000, 'the last step above the floor'],
    [10_000, 100_000, 'pinned at the floor the moment it is reached'],
    [10_001, 100_000, 'still pinned one ms later'],
    [999_999, 100_000, 'still pinned far past floor time'],
    [-1, 300_000, 'a negative elapsed clamps to the start price, never above it'],
    [Number.MAX_SAFE_INTEGER, 100_000, 'an astronomically large elapsed still answers the floor'],
  ]

  it.each(cases)('answers %d ms of elapsed as %d (%s)', (elapsed, expected, _label) => {
    expect(dutchPrice(MECHANICS, elapsed)).toBe(expected)
  })

  it('never answers a price above the start, whatever the elapsed', () => {
    for (const elapsed of [0, 1, 999, 10_000, Number.MAX_SAFE_INTEGER]) {
      expect(dutchPrice(MECHANICS, elapsed)).toBeLessThanOrEqual(MECHANICS.startPrice)
    }
  })

  it('steps on a span the decrement does not divide, and pins at the floor', () => {
    const mechanics: DutchMechanics = {
      startPrice: 100_000,
      floorPrice: 40_000,
      decrementStep: 30_000,
      decrementIntervalMs: 1_000,
    }
    expect(dutchPrice(mechanics, 1_000)).toBe(70_000)
    expect(dutchPrice(mechanics, 2_000)).toBe(40_000)
    expect(dutchPrice(mechanics, 3_000)).toBe(40_000)
  })

  it('stands still at the start when the floor IS the start (a legal row)', () => {
    const flat: DutchMechanics = { ...MECHANICS, floorPrice: 300_000 }
    expect(dutchPrice(flat, 0)).toBe(300_000)
    expect(dutchPrice(flat, 60_000)).toBe(300_000)
  })

  it('mirrors the server for a row the live-start guard refuses: a floor above the start', () => {
    // D-92 refuses to open such a row, so no body ever sees one — but the
    // client must never disagree with the server's own total answer, so this
    // pins the server's branch (no steps, then the floor clamp) rather than
    // inventing a second rule.
    expect(dutchPrice({ ...MECHANICS, floorPrice: 400_000 }, 5_000)).toBe(400_000)
  })

  it('answers the start price for mechanics with no usable clock', () => {
    // The number that is not a positive step or interval — NaN included — has no
    // clock: the module must not divide by zero or freeze a projector.
    for (const broken of [
      { ...MECHANICS, decrementStep: 0 },
      { ...MECHANICS, decrementStep: -1 },
      { ...MECHANICS, decrementIntervalMs: 0 },
      { ...MECHANICS, decrementIntervalMs: Number.NaN },
    ]) {
      expect(dutchPrice(broken, 5_000)).toBe(MECHANICS.startPrice)
    }
  })
})

describe('atFloor (D-103)', () => {
  it('is true exactly while the price is pinned at the floor', () => {
    expect(atFloor(100_000, 100_000)).toBe(true)
    expect(atFloor(120_000, 100_000)).toBe(false)
  })

  it('is true at the start when the floor IS the start', () => {
    expect(atFloor(300_000, 300_000)).toBe(true)
  })

  it('treats a price below the floor as pinned rather than as still falling', () => {
    expect(atFloor(90_000, 100_000)).toBe(true)
  })
})

describe('canAccept (D-104/DTCH-02)', () => {
  const open = { status: 'open', inFlight: false, price: 200_000, budget: 1_000_000 } as const

  it('enables the CTA only while the socket is open, nothing is in flight and the price is affordable', () => {
    expect(canAccept(open)).toBe(true)
  })

  it('treats equality as affordable — the server charges a price equal to the budget', () => {
    expect(canAccept({ ...open, price: 1_000_000, budget: 1_000_000 })).toBe(true)
  })

  it('refuses one VND above the budget', () => {
    expect(canAccept({ ...open, price: 1_000_001, budget: 1_000_000 })).toBe(false)
  })

  it('re-derives as the price falls: disabled above the budget, enabled the moment it reaches it', () => {
    // D-104: the predicate is asked again on every render, so the CTA re-enables
    // by itself inside one render tick as the price falls past the budget — no
    // snapshot and no second decision rule.
    const budget = 250_000
    expect(canAccept({ ...open, price: 300_000, budget })).toBe(false)
    expect(canAccept({ ...open, price: 260_000, budget })).toBe(false)
    expect(canAccept({ ...open, price: 250_000, budget })).toBe(true)
  })

  it('never enables while the socket cannot deliver', () => {
    for (const status of ['connecting', 'lost', 'superseded'] as const) {
      expect(canAccept({ ...open, status })).toBe(false)
    }
  })

  it('never enables while an accept is in flight', () => {
    expect(canAccept({ ...open, inFlight: true })).toBe(false)
  })
})

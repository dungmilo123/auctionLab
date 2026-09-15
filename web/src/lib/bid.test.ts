import { describe, expect, it } from 'vitest'
import {
  QUICK_BID_DELTAS,
  bidInputValue,
  bidReasonKey,
  isOutbid,
  isOutOfBudget,
  parseBidAmount,
  quickBidAmount,
} from './bid'

describe('the quick-bid arithmetic (D-71, ENGL-01)', () => {
  it('carries exactly the three fixed steps', () => {
    expect([...QUICK_BID_DELTAS]).toEqual([10_000, 50_000, 100_000])
  })

  it('adds each delta to the SERVER-provided minimum', () => {
    expect(quickBidAmount(1_000_000, 10_000)).toBe(1_010_000)
    expect(quickBidAmount(1_000_000, 50_000)).toBe(1_050_000)
    expect(quickBidAmount(1_000_000, 100_000)).toBe(1_100_000)
  })

  it('never invents a ladder of its own — the minimum is the base', () => {
    expect(quickBidAmount(0, 50_000)).toBe(50_000)
    expect(quickBidAmount(5, 100_000)).toBe(100_005)
  })
})

describe('parseBidAmount (D-71)', () => {
  const accepted: Array<[raw: string, expected: number]> = [
    ['1500000', 1_500_000],
    ['0', 0],
    ['000123', 123],
    ['9007199254740991', Number.MAX_SAFE_INTEGER],
  ]

  it.each(accepted)('parses the raw digit string %s as %d', (raw, expected) => {
    expect(parseBidAmount(raw)).toBe(expected)
  })

  const rejected: Array<[label: string, raw: string]> = [
    ['an empty field', ''],
    ['letters', 'abc'],
    ['grouped VND input', '1.000'],
    ['a decimal fraction', '1000.5'],
    ['whitespace around digits', ' 1000'],
    ['a trailing space', '1000 '],
    ['a plus sign', '+1000'],
    ['a negative amount', '-1000'],
    ['scientific notation', '1e5'],
    ['a value beyond safe-integer precision', '9007199254740993'],
  ]

  it.each(rejected)('rejects %s', (_label, raw) => {
    expect(parseBidAmount(raw)).toBeNull()
  })
})

describe('bidReasonKey (D-52/D-53)', () => {
  const known: Array<[reason: string, key: string]> = [
    ['invalid_amount', 'reason.invalidAmount'],
    ['window_closed', 'reason.windowClosed'],
    ['below_minimum', 'reason.belowMinimum'],
    ['exceeds_budget', 'reason.exceedsBudget'],
    ['already_leading', 'reason.alreadyLeading'],
  ]

  it.each(known)('maps the %s rejection to its copy key', (reason, key) => {
    expect(bidReasonKey(reason)).toBe(key)
  })

  it('falls back to the generic error copy for anything unknown', () => {
    expect(bidReasonKey('a_reason_from_a_later_phase')).toBe('error.generic')
    expect(bidReasonKey('')).toBe('error.generic')
  })
})

describe('bidInputValue (D-71)', () => {
  it('follows the server minimum while the player has not edited the field', () => {
    expect(bidInputValue(false, '', 1_000_000)).toBe('1000000')
    expect(bidInputValue(false, '1300000', 1_200_000)).toBe('1200000')
  })

  it('never overwrites a hand-edited value when the minimum moves', () => {
    expect(bidInputValue(true, '1300000', 1_200_000)).toBe('1300000')
    expect(bidInputValue(true, '', 1_200_000)).toBe('')
  })
})

describe('isOutbid (D-72)', () => {
  const live = { phase: 'LIVE', lastBid: 1_000_000, isTopBidder: false, remainingMs: 5_000 }

  it('shows the banner only when the server says the player is not top', () => {
    expect(isOutbid(live)).toBe(true)
  })

  it('clears the banner the moment the player is top again', () => {
    expect(isOutbid({ ...live, isTopBidder: true })).toBe(false)
  })

  it('clears when the window has closed', () => {
    expect(isOutbid({ ...live, remainingMs: 0 })).toBe(false)
  })

  it('never shows for a player who has not bid in this run', () => {
    expect(isOutbid({ ...live, lastBid: null })).toBe(false)
  })

  it('never shows outside LIVE, whatever the last bid was', () => {
    expect(isOutbid({ ...live, phase: 'SETTLED' })).toBe(false)
    expect(isOutbid({ ...live, phase: 'ITEM_INTRO' })).toBe(false)
  })
})

describe('isOutOfBudget (ECON-01)', () => {
  it('disables the controls when the next valid bid exceeds the budget', () => {
    expect(isOutOfBudget(5_000, 6_000)).toBe(true)
  })

  it('keeps them enabled when the budget covers the minimum exactly', () => {
    expect(isOutOfBudget(6_000, 6_000)).toBe(false)
    expect(isOutOfBudget(7_000, 6_000)).toBe(false)
  })
})

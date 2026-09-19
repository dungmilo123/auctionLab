// The Players list renders LOCAL wall-clock time (D-80), so the only way the
// literal expectations below are about THE FUNCTION rather than about this
// machine is to pin the process timezone — here, at the top of the file and
// before any Date call in the suite. No Vitest config is added for it, and no
// `Intl` is involved: the venue may be fully offline (03-RESEARCH Pitfall 9).
// The app tsconfig carries only `vite/client` types, so the one Node global
// this file needs is declared rather than pulled in as a dependency.
declare const process: { env: { TZ?: string } }
process.env.TZ = 'UTC'

import { describe, expect, it } from 'vitest'
import { formatTimeOfDay, formatUSD } from './format'

// The wire and the database both carry whole integers; the screen shows USD
// with comma grouping and two decimals (D-02).
const cases: Array<[amount: number, expected: string]> = [
  [0, '$0.00'],
  [7, '$7.00'],
  [999, '$999.00'],
  [1000, '$1,000.00'],
  [1234, '$1,234.00'],
  [1000000, '$1,000,000.00'],
  [1500000, '$1,500,000.00'],
  [1234567890, '$1,234,567,890.00'],
  [-1234.56, '-$1,234.00'],
]

describe('formatUSD', () => {
  it.each(cases)('renders %d as %s', (amount, expected) => {
    expect(formatUSD(amount)).toBe(expected)
  })

  it('groups every three digits regardless of how many there are', () => {
    expect(formatUSD(12_345_678)).toBe('$12,345,678.00')
  })

  it('puts the sign before the dollar symbol, never after', () => {
    expect(formatUSD(-1)).toBe('-$1.00')
    expect(formatUSD(-1_000_000)).toBe('-$1,000,000.00')
    expect(formatUSD(1_000_000)).toBe('$1,000,000.00')
  })

  it('always renders exactly two decimals, even on a whole amount', () => {
    for (const amount of [0, 1, 999, 1_000_000, -5]) {
      expect(formatUSD(amount)).toMatch(/^-?\$[\d,]+\.\d{2}$/)
      expect(formatUSD(amount).endsWith('.00')).toBe(true)
    }
  })

  it('truncates a fractional input toward zero, so an amount keeps its sign', () => {
    expect(formatUSD(1234.99)).toBe('$1,234.00')
    expect(formatUSD(-1234.01)).toBe('-$1,234.00')
  })
})

describe('formatTimeOfDay (D-80)', () => {
  it('renders a zero-padded 24-hour local time', () => {
    expect(formatTimeOfDay(Date.UTC(2026, 8, 16, 14, 32))).toBe('14:32')
    expect(formatTimeOfDay(Date.UTC(2026, 0, 2, 9, 7))).toBe('09:07')
    expect(formatTimeOfDay(Date.UTC(2026, 11, 31, 0, 5))).toBe('00:05')
  })

  it('drops the seconds and never renders a date', () => {
    expect(formatTimeOfDay(Date.UTC(2026, 8, 16, 23, 59) + 59_000 + 999)).toBe('23:59')
    expect(formatTimeOfDay(Date.UTC(2026, 8, 16, 14, 32))).not.toContain('2026')
  })
})

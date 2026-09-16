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
import { formatTimeOfDay, formatVND } from './format'

// The wire and the database both carry integers; the screen shows VND with dots.
const cases: Array<[amount: number, expected: string]> = [
  [0, '0'],
  [7, '7'],
  [999, '999'],
  [1000, '1.000'],
  [1234, '1.234'],
  [1000000, '1.000.000'],
  [1500000, '1.500.000'],
  [1234567890, '1.234.567.890'],
]

describe('formatVND', () => {
  it.each(cases)('renders %d as %s', (amount, expected) => {
    expect(formatVND(amount)).toBe(expected)
  })

  it('groups every three digits regardless of how many there are', () => {
    expect(formatVND(12_345_678)).toBe('12.345.678')
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

import { describe, expect, it } from 'vitest'
import { formatVND } from './format'

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

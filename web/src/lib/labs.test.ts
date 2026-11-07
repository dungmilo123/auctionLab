import { describe, expect, it } from 'vitest'
import {
  applySoftClose, centsToDong, dongBidToCents, formatDongAmount, formatUsd, placeProxy,
  type FxRate, type ProxyState, type SoftClose,
} from './labs'

const rate: FxRate = { id: 1, dongPerUsd: 25_380, source: 'operator', effectiveAt: 0 }

describe('multi-currency display', () => {
  it('converts cents to whole đồng with half-up rounding', () => {
    expect(centsToDong(17_500, rate)).toBe(4_441_500n)
    expect(centsToDong(1, rate)).toBe(254n) // 253.8 → 254
    expect(centsToDong(17_500, rate, 1000)).toBe(4_442_000n)
  })
  it('formats both currencies', () => {
    expect(formatUsd(123_456_78)).toBe('$123,456.78')
    expect(formatDongAmount(4_441_500n)).toBe('4.441.500 ₫')
  })
  it('rounds a đồng bid up to the next valid USD increment', () => {
    // 4,400,000 ₫ ≈ $173.37 → min $170, increment $5 → $175
    expect(dongBidToCents(4_400_000, rate, 17_000, 500)).toBe(17_500)
    expect(dongBidToCents(100, rate, 17_000, 500)).toBe(17_000)
    expect(dongBidToCents(0, rate, 17_000, 500)).toBeNull()
  })
})

describe('soft close', () => {
  const base: SoftClose = { closeAt: 600_000, originalCloseAt: 600_000, windowMs: 120_000, capMs: 300_000, extensions: [] }
  it('ignores bids outside the window', () => {
    expect(applySoftClose(base, 400_000)).toBe(base)
  })
  it('extends to bid + window inside the window', () => {
    const s = applySoftClose(base, 550_000)
    expect(s.closeAt).toBe(670_000)
    expect(s.extensions).toHaveLength(1)
  })
  it('never extends past the cap', () => {
    const s = applySoftClose({ ...base, closeAt: 890_000 }, 850_000)
    expect(s.closeAt).toBe(900_000)
  })
  it('ignores bids after the close', () => {
    expect(applySoftClose(base, 600_000)).toBe(base)
  })
})

describe('proxy bidding', () => {
  const empty: ProxyState = { price: null, start: 10_000, increment: 500, leader: null, history: [] }
  const ok = (s: ReturnType<typeof placeProxy>) => {
    if ('error' in s) throw new Error(s.error)
    return s
  }
  it('opens at the starting price whatever the max', () => {
    const s = ok(placeProxy(empty, { bidder: 'you', max: 25_000, placedAt: 1 }))
    expect(s.price).toBe(10_000)
  })
  it('leader auto-responds one increment above a lower challenger', () => {
    const a = ok(placeProxy(empty, { bidder: 'you', max: 25_000, placedAt: 1 }))
    const b = ok(placeProxy(a, { bidder: 'rival', max: 18_000, placedAt: 2 }))
    expect(b.leader?.bidder).toBe('you')
    expect(b.price).toBe(18_500)
  })
  it('a higher challenger takes the lead at one increment over the old max', () => {
    const a = ok(placeProxy(empty, { bidder: 'you', max: 25_000, placedAt: 1 }))
    const b = ok(placeProxy(a, { bidder: 'rival', max: 40_000, placedAt: 2 }))
    expect(b.leader?.bidder).toBe('rival')
    expect(b.price).toBe(25_500)
  })
  it('equal maxima: the earlier bid wins at the tied amount', () => {
    const a = ok(placeProxy(empty, { bidder: 'you', max: 25_000, placedAt: 1 }))
    const b = ok(placeProxy(a, { bidder: 'rival', max: 25_000, placedAt: 2 }))
    expect(b.leader?.bidder).toBe('you')
    expect(b.price).toBe(25_000)
  })
  it('rejects bids below the next minimum', () => {
    const a = ok(placeProxy(empty, { bidder: 'you', max: 25_000, placedAt: 1 }))
    expect(placeProxy(a, { bidder: 'rival', max: 10_000, placedAt: 2 })).toEqual({ error: 'too_low' })
  })
  it('raising your own max does not move the price', () => {
    const a = ok(placeProxy(empty, { bidder: 'you', max: 25_000, placedAt: 1 }))
    const b = ok(placeProxy(a, { bidder: 'you', max: 30_000, placedAt: 2 }))
    expect(b.price).toBe(10_000)
    expect(b.leader?.max).toBe(30_000)
  })
})

describe('revenue forecast', () => {
  it('predicts a hammer band of current bid +30–40%', async () => {
    const { predictHammer } = await import('./labs')
    expect(predictHammer(10_000, 30, 40)).toEqual({ low: 13_000, mid: 13_500, high: 14_000 })
    expect(predictHammer(333, 30, 40)).toEqual({ low: 433, mid: 450, high: 466 })
  })
  it('adds open-lot bands on top of realised revenue', async () => {
    const { forecastRevenue } = await import('./labs')
    const f = forecastRevenue(50_000, [{ currentCents: 10_000, closesInDays: 1 }, { currentCents: 20_000, closesInDays: 2 }], 30, 40)
    expect(f).toEqual({ low: 89_000, mid: 90_500, high: 92_000 })
  })
  it('collapses to realised revenue when nothing is open', async () => {
    const { forecastRevenue } = await import('./labs')
    expect(forecastRevenue(50_000, [], 30, 40)).toEqual({ low: 50_000, mid: 50_000, high: 50_000 })
  })
})

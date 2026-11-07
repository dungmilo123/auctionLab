/**
 * Feature-lab logic: pure, fixture-only models for the /admin/labs prototypes.
 * Nothing here touches the wire, the schema or a stored amount. Money stays in
 * integer USD cents; VND is a display transform at an explicit rate.
 */

// ---------- 1. Multi-currency display ----------

export type DisplayCurrency = 'USD' | 'VND'
/** An operator-entered rate snapshot: whole đồng per one US dollar. */
export type FxRate = { id: number; dongPerUsd: number; source: string; effectiveAt: number }

const group = (n: bigint, sep: string) => n.toString().replace(/\B(?=(\d{3})+(?!\d))/g, sep)

/** Cents → whole đồng, half-up. VND has no minor unit (ISO 4217 exponent 0). */
export function centsToDong(cents: number, rate: FxRate, roundTo = 1): bigint {
  const raw = BigInt(cents) * BigInt(rate.dongPerUsd)
  const step = 100n * BigInt(roundTo)
  return ((raw + step / 2n) / step) * BigInt(roundTo)
}

export function formatUsd(cents: number): string {
  const n = BigInt(cents)
  return `$${group(n / 100n, ',')}.${(n % 100n).toString().padStart(2, '0')}`
}

export function formatDongAmount(dong: bigint): string {
  return `${group(dong, '.')} ₫`
}

/**
 * A VND bid is converted at the current rate and rounded UP to the next valid
 * USD increment above the minimum, so the bidder never pays less than typed.
 */
export function dongBidToCents(dong: number, rate: FxRate, minimumCents: number, incrementCents: number): number | null {
  if (!Number.isSafeInteger(dong) || dong <= 0) return null
  const exact = Math.ceil((dong * 100) / rate.dongPerUsd)
  if (exact <= minimumCents) return minimumCents
  return minimumCents + Math.ceil((exact - minimumCents) / incrementCents) * incrementCents
}

// ---------- 2. Soft close (anti-sniping) ----------

export type SoftClose = {
  closeAt: number
  originalCloseAt: number
  windowMs: number
  capMs: number
  extensions: { at: number; from: number; to: number }[]
}

/** A bid inside the final window pushes the close to bid + window, never past the cap. */
export function applySoftClose(s: SoftClose, bidAt: number): SoftClose {
  if (bidAt >= s.closeAt) return s
  const remaining = s.closeAt - bidAt
  if (remaining >= s.windowMs) return s
  const to = Math.min(bidAt + s.windowMs, s.originalCloseAt + s.capMs)
  if (to <= s.closeAt) return s
  return { ...s, closeAt: to, extensions: [...s.extensions, { at: bidAt, from: s.closeAt, to }] }
}

// ---------- 3. Proxy bidding ----------

export type ProxyBid = { bidder: string; max: number; placedAt: number }
export type ProxyEvent = { bidder: string; amount: number; auto: boolean }
export type ProxyState = {
  price: number | null
  start: number
  increment: number
  leader: ProxyBid | null
  history: ProxyEvent[]
}

export const minimumNext = (s: ProxyState) => (s.price === null ? s.start : s.price + s.increment)

/**
 * eBay-style resolution. The higher maximum leads at one increment above the
 * lower maximum (capped at its own max). Equal maxima: the earlier bid leads
 * at the tied amount. A bidder raising their own max never moves the price.
 */
export function placeProxy(s: ProxyState, bid: ProxyBid): ProxyState | { error: 'too_low' } {
  if (s.leader?.bidder === bid.bidder) {
    return bid.max > s.leader.max ? { ...s, leader: { ...s.leader, max: bid.max } } : { error: 'too_low' }
  }
  if (bid.max < minimumNext(s)) return { error: 'too_low' }
  const leader = s.leader
  if (!leader) {
    const price = s.start
    return { ...s, price, leader: bid, history: [...s.history, { bidder: bid.bidder, amount: price, auto: false }] }
  }
  const history = [...s.history]
  if (bid.max > leader.max) {
    history.push({ bidder: leader.bidder, amount: leader.max, auto: true })
    const price = Math.min(bid.max, leader.max + s.increment)
    history.push({ bidder: bid.bidder, amount: price, auto: price !== bid.max })
    return { ...s, price, leader: bid, history }
  }
  // Challenger loses (or ties and is later): leader auto-responds.
  history.push({ bidder: bid.bidder, amount: bid.max, auto: false })
  const price = bid.max === leader.max ? leader.max : Math.min(leader.max, bid.max + s.increment)
  history.push({ bidder: leader.bidder, amount: price, auto: true })
  return { ...s, price, history }
}

// ---------- Fixtures ----------

/** Seed catalogue lots (internal/catalog/seed_data.json) with their bundled art. */
export const SEED_LOTS = {
  rangefinder: { n: '02', title: 'Silver rangefinder study', event: 'Analog Memories' },
  fieldWatch: { n: '07', title: 'Field watch with canvas strap', event: 'Objects of Time' },
  turntable: { n: '17', title: 'Belt drive turntable', event: 'The Listening Room' },
  twinLens: { n: '03', title: 'Twin lens portrait camera', event: 'Analog Memories' },
  pocketWatch: { n: '08', title: 'Open face pocket watch', event: 'Objects of Time' },
  mantelClock: { n: '11', title: 'Oak mantel clock', event: 'Objects of Time' },
  jazzRecord: { n: '13', title: 'Midnight jazz record', event: 'The Listening Room' },
  receiver: { n: '16', title: 'Silver stereo receiver', event: 'The Listening Room' },
  speakers: { n: '18', title: 'Walnut bookshelf speakers', event: 'The Listening Room' },
} as const
export type SeedLot = (typeof SEED_LOTS)[keyof typeof SEED_LOTS]
export const lotArt = (lot: SeedLot, detail = false) => `/site-assets/catalog/lot-${lot.n}${detail ? '-detail' : ''}.svg`

// ---------- 4. Revenue forecast (rules, not ML) ----------

export type OpenLot = { currentCents: number; closesInDays: number }
export type Forecast = { low: number; mid: number; high: number }

/**
 * Predicted hammer for an open lot = current bid × (1 + uplift), as a band.
 * Integer cents, half-up; the band is [low, high] with its midpoint.
 */
export function predictHammer(currentCents: number, lowPct: number, highPct: number): Forecast {
  const at = (pct: number) => Math.round((currentCents * (100 + pct)) / 100)
  return { low: at(lowPct), mid: at((lowPct + highPct) / 2), high: at(highPct) }
}

/** Realised revenue plus each open lot's predicted band, summed. */
export function forecastRevenue(realisedCents: number, open: OpenLot[], lowPct: number, highPct: number): Forecast {
  return open.reduce<Forecast>((t, o) => {
    const p = predictHammer(o.currentCents, lowPct, highPct)
    return { low: t.low + p.low, mid: t.mid + p.mid, high: t.high + p.high }
  }, { low: realisedCents, mid: realisedCents, high: realisedCents })
}

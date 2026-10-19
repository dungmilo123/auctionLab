import type { Category, PublicEvent, PublicLot } from '@/api/catalog'
import { validInstant } from '@/api/public-catalog'
export const phases = ['all', 'upcoming', 'window', 'elapsed'] as const
export const sorts = [
  'closing-soon',
  'price-asc',
  'price-desc',
  'title',
] as const
export type Phase = Exclude<(typeof phases)[number], 'all'> | 'unavailable'
export function phase(
  open: number | null,
  close: number | null,
  now: number,
): Phase {
  if (
    !validInstant(open) ||
    !validInstant(close) ||
    close <= open ||
    !validInstant(Math.floor(now))
  )
    return 'unavailable'
  return now < open ? 'upcoming' : now < close ? 'window' : 'elapsed'
}
export function sampleTime(
  sample: { serverTime: number; receivedAt: number },
  monotonic: number,
): number {
  return sample.serverTime + Math.max(0, monotonic - sample.receivedAt)
}
export function query(search: string, categories: Category[]) {
  const p = new URLSearchParams(search)
  const rawPhase = p.get('phase'),
    rawSort = p.get('sort')
  return {
    q: (p.get('q') ?? '').trim(),
    category: categories.find((c) => c.slug === p.get('category'))?.slug ?? '',
    phase: phases.find((v) => v === rawPhase) ?? 'all',
    sort: sorts.find((v) => v === rawSort) ?? 'closing-soon',
  }
}
export const textOrder = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0)
const nullable = (a: number | null, b: number | null, direction = 1) =>
  a === null ? (b === null ? 0 : 1) : b === null ? -1 : direction * (a - b)
export function closingOrder(a: PublicLot, b: PublicLot, now: number) {
  const x = validInstant(a.closes_at) ? a.closes_at : null,
    y = validInstant(b.closes_at) ? b.closes_at : null
  if (x === null || y === null) return nullable(x, y)
  if (x > now !== y > now) return x > now ? -1 : 1
  return x > now ? x - y : y - x
}
export function selectLots(
  lots: PublicLot[],
  selection: ReturnType<typeof query>,
  categories: Category[],
  now: number,
) {
  const categoryID = categories.find((c) => c.slug === selection.category)?.id
  return lots
    .filter(
      (l) =>
        (!selection.q ||
          [l.title, l.description, l.brand ?? '', l.era ?? ''].some((t) =>
            t.toLowerCase().includes(selection.q.toLowerCase()),
          )) &&
        (!categoryID || l.category_id === categoryID) &&
        (selection.phase === 'all' ||
          phase(l.opens_at, l.closes_at, now) === selection.phase),
    )
    .sort(
      (a, b) =>
        (selection.sort === 'title'
          ? textOrder(a.title.toLowerCase(), b.title.toLowerCase())
          : selection.sort === 'price-asc' || selection.sort === 'price-desc'
            ? nullable(
                a.starting_price_minor,
                b.starting_price_minor,
                selection.sort === 'price-asc' ? 1 : -1,
              )
            : closingOrder(a, b, now)) || textOrder(a.slug, b.slug),
    )
}
export function orderEvents(events: PublicEvent[], now: number) {
  const rank = { window: 0, upcoming: 1, elapsed: 2, unavailable: 3 }
  return [...events].sort((a, b) => {
    const ap = phase(a.opens_at, a.planned_end, now),
      bp = phase(b.opens_at, b.planned_end, now)
    return (
      rank[ap] - rank[bp] ||
      nullable(a.opens_at, b.opens_at, ap === 'elapsed' ? -1 : 1) ||
      textOrder(a.slug, b.slug)
    )
  })
}
export function homeSelection(events: PublicEvent[], now: number) {
  return {
    featured: orderEvents(events, now).slice(0, 3),
    nearest: events
      .flatMap((e) => e.lots)
      .filter((l) => phase(l.opens_at, l.closes_at, now) === 'window')
      .sort((a, b) => closingOrder(a, b, now) || textOrder(a.slug, b.slug))
      .slice(0, 6),
  }
}

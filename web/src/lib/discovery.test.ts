import { expect, it, vi, afterEach } from 'vitest'
import type { PublicLot, PublicEvent, Category } from '@/api/catalog'
import { publicCatalog } from '@/api/public-catalog'
import {
  query,
  phase,
  sampleTime,
  selectLots,
  orderEvents,
  homeSelection,
} from './discovery'
export const categories: Category[] = [
  {
    id: 'c',
    slug: 'film-cameras',
    name_en: 'Cameras',
    name_vi: 'Máy ảnh',
    sort_order: 0,
  },
]
export const lot = (
  slug: string,
  extra: Partial<PublicLot> = {},
): PublicLot => ({
  id: slug,
  slug,
  title: slug,
  description: 'A camera',
  category_id: 'c',
  brand: null,
  era: null,
  story: null,
  condition: 'Used',
  images: [{ path: '/site-assets/image-unavailable.svg', alt: 'Camera' }],
  specifications: [],
  fulfillment: {
    shipping_enabled: true,
    pickup_enabled: true,
    shipping_fee_minor: 0,
    shipping_instructions: 'Mail',
    pickup_instructions: 'Collect',
  },
  starting_price_minor: 0,
  bid_increment_minor: 101,
  currency: 'USD',
  event_id: 'e',
  event_slug: 'event',
  position: 0,
  opens_at: 100,
  closes_at: 200,
  state: 'published',
  demo: false,
  ...extra,
})
export const event = (
  slug: string,
  extra: Partial<PublicEvent> = {},
): PublicEvent => ({
  id: slug,
  slug,
  title: slug,
  theme: 'Theme',
  description: 'Description',
  cover_path: '/site-assets/image-unavailable.svg',
  cover_alt: 'Cover',
  opens_at: 100,
  first_closes_at: 200,
  close_interval_ms: 10,
  planned_end: 250,
  state: 'published',
  demo: true,
  lots: [lot(slug)],
  ...extra,
})
afterEach(() => vi.unstubAllGlobals())
it('restores shared queries, first duplicates, defaults and plain text', () => {
  expect(
    query(
      '?q=%20Canon%20&category=film-cameras&phase=upcoming&sort=price-asc',
      categories,
    ),
  ).toEqual({
    q: 'Canon',
    category: 'film-cameras',
    phase: 'upcoming',
    sort: 'price-asc',
  })
  expect(
    query(
      '?q=%3Cb%3E&q=other&category=bad&category=film-cameras&phase=bad&phase=window&sort=bad&unknown=x',
      categories,
    ),
  ).toEqual({ q: '<b>', category: '', phase: 'all', sort: 'closing-soon' })
})
it('uses inclusive opening and exclusive close, each endpoint and safe times', () => {
  expect([99, 100, 199, 200].map((n) => phase(100, 200, n))).toEqual([
    'upcoming',
    'window',
    'window',
    'elapsed',
  ])
  expect(phase(100, 250, 200)).toBe('window')
  for (const [o, c] of [
    [null, 200],
    [100, null],
    [100, 100],
    [-1, 200],
    [1.2, 200],
    [0, Infinity],
    [0, 8640000000000001],
  ])
    expect(phase(o!, c!, 150)).toBe('unavailable')
  vi.spyOn(Date, 'now').mockReturnValue(9999999999999)
  expect(sampleTime({ serverTime: 100, receivedAt: 200 }, 250)).toBe(150)
  expect(sampleTime({ serverTime: 100, receivedAt: 200 }, 100)).toBe(100)
  vi.restoreAllMocks()
})
it('combines text, category and phase; optional text, no matches and exact cents', () => {
  const lots = [
    lot('b', { brand: 'CANON', starting_price_minor: 101 }),
    lot('a', { era: 'Canon era', starting_price_minor: 0 }),
    lot('z', { category_id: 'other', brand: 'Canon' }),
    lot('d', { brand: 'Canon', closes_at: 140 }),
  ]
  const selection = query(
    '?q=canon&category=film-cameras&phase=window&sort=price-asc',
    categories,
  )
  expect(
    selectLots(lots, selection, categories, 150).map((l) => l.slug),
  ).toEqual(['a', 'b'])
  expect(
    selectLots(lots, { ...selection, sort: 'price-desc' }, categories, 150).map(
      (l) => l.slug,
    ),
  ).toEqual(['b', 'a'])
  expect(
    selectLots(lots, { ...selection, q: 'missing' }, categories, 150),
  ).toEqual([])
  expect(
    selectLots([lot('x')], { ...selection, q: 'canon' }, categories, 150),
  ).toEqual([])
})
it('sorts future before elapsed, descending elapsed, null last and slug ties', () => {
  const lots = [
    lot('old', { closes_at: 120 }),
    lot('recent', { closes_at: 140 }),
    lot('future', { opens_at: 160, closes_at: 180 }),
    lot('b'),
    lot('a'),
    lot('null', { closes_at: null, starting_price_minor: null }),
  ]
  expect(
    selectLots(lots, query('', categories), categories, 150).map((l) => l.slug),
  ).toEqual(['future', 'a', 'b', 'recent', 'old', 'null'])
  for (const sort of ['price-asc', 'price-desc'])
    expect(
      selectLots(lots, query('?sort=' + sort, categories), categories, 150).at(
        -1,
      )?.slug,
    ).toBe('null')
  expect(
    selectLots(
      [
        lot('b', { title: 'ALPHA' }),
        lot('a', { title: 'alpha' }),
        lot('c', { title: 'Beta' }),
      ],
      query('?sort=title', categories),
      categories,
      150,
    ).map((l) => l.slug),
  ).toEqual(['a', 'b', 'c'])
})
it('orders overlapping events, ties and current-window home lots with limits', () => {
  const events = [
    event('elapsed-old', { opens_at: 0, planned_end: 90 }),
    event('upcoming', { opens_at: 200, planned_end: 300 }),
    event('b'),
    event('a'),
    event('elapsed-recent', { opens_at: 50, planned_end: 99 }),
    event('unknown', { planned_end: null }),
  ]
  expect(orderEvents(events, 150).map((e) => e.slug)).toEqual([
    'a',
    'b',
    'upcoming',
    'elapsed-recent',
    'elapsed-old',
    'unknown',
  ])
  const selection = homeSelection(
    [
      event('many', {
        lots: Array.from({ length: 9 }, (_, i) =>
          lot(String(i), { closes_at: 200 + i }),
        ),
      }),
      ...events,
    ],
    150,
  )
  expect(selection.featured).toHaveLength(3)
  expect(selection.nearest).toHaveLength(6)
  expect(homeSelection([], 150)).toEqual({ featured: [], nearest: [] })
  expect(
    homeSelection(
      [event('future', { lots: [lot('future', { opens_at: 160 })] })],
      150,
    ).nearest,
  ).toEqual([])
})
it('accepts complete public lot/event details and validates nested content', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((path: string) =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              data: path.includes('/events/') ? event('e') : lot('l'),
              meta: { server_time: 100 },
            }),
          ),
        ),
      ),
  )
  expect((await publicCatalog.lot('l')).data.starting_price_minor).toBe(0)
  expect((await publicCatalog.event('e')).data.lots).toHaveLength(1)
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({
            data: lot('l', { fulfillment: null as never }),
            meta: { server_time: 100 },
          }),
        ),
      ),
  )
  await expect(publicCatalog.lot('l')).rejects.toMatchObject({
    code: 'invalid_response',
  })
})

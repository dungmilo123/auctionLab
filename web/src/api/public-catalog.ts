import {
  CatalogError,
  type Category,
  type PublicEvent,
  type PublicLot,
} from './catalog'
import { MAX_CATALOG_INSTANT } from '@/lib/catalog-schedule'
import { MAX_CATALOG_MONEY } from '@/lib/catalog-money'

export type PublicResponse<T> = {
  data: T
  serverTime: number
  receivedAt: number
}
type Obj = Record<string, unknown>
const object = (v: unknown): v is Obj =>
  typeof v === 'object' && v !== null && !Array.isArray(v)
const text = (v: unknown): v is string => typeof v === 'string'
const optionalText = (v: unknown) => v === null || text(v)
export const validInstant = (v: unknown): v is number =>
  Number.isSafeInteger(v) &&
  (v as number) >= 0 &&
  (v as number) <= MAX_CATALOG_INSTANT
const money = (v: unknown) =>
  v === null ||
  (Number.isSafeInteger(v) &&
    (v as number) >= 0 &&
    (v as number) <= MAX_CATALOG_MONEY)
// Malformed schedule endpoints remain inspectable with an unavailable badge.
const timing = (v: unknown) => v === null || typeof v === 'number'
const texts = (v: Obj, keys: string[]) => keys.every((k) => text(v[k]))
const category = (v: unknown): v is Category =>
  object(v) &&
  texts(v, ['id', 'slug', 'name_en', 'name_vi']) &&
  Number.isSafeInteger(v.sort_order)
const lot = (v: unknown): v is PublicLot =>
  object(v) &&
  texts(v, [
    'id',
    'slug',
    'title',
    'description',
    'condition',
    'event_id',
    'event_slug',
  ]) &&
  v.state === 'published' &&
  typeof v.demo === 'boolean' &&
  v.currency === 'USD' &&
  ['brand', 'era', 'story', 'category_id'].every((k) => optionalText(v[k])) &&
  ['opens_at', 'closes_at'].every((k) => timing(v[k])) &&
  Number.isSafeInteger(v.position) &&
  money(v.starting_price_minor) &&
  money(v.bid_increment_minor) &&
  Array.isArray(v.images) &&
  v.images.every((i) => object(i) && texts(i, ['path', 'alt'])) &&
  Array.isArray(v.specifications) &&
  v.specifications.every((i) => object(i) && texts(i, ['name', 'value'])) &&
  object(v.fulfillment) &&
  typeof v.fulfillment.shipping_enabled === 'boolean' &&
  typeof v.fulfillment.pickup_enabled === 'boolean' &&
  money(v.fulfillment.shipping_fee_minor) &&
  texts(v.fulfillment, ['shipping_instructions', 'pickup_instructions'])
const event = (v: unknown): v is PublicEvent =>
  object(v) &&
  texts(v, [
    'id',
    'slug',
    'title',
    'theme',
    'description',
    'cover_path',
    'cover_alt',
  ]) &&
  v.state === 'published' &&
  typeof v.demo === 'boolean' &&
  ['opens_at', 'first_closes_at', 'planned_end', 'close_interval_ms'].every(
    (k) => timing(v[k]),
  ) &&
  Array.isArray(v.lots) &&
  v.lots.every(lot)
const list =
  <T>(check: (v: unknown) => v is T) =>
  (v: unknown): v is T[] =>
    Array.isArray(v) && v.every(check)
async function read<T>(
  path: string,
  check: (v: unknown) => v is T,
  signal?: AbortSignal,
): Promise<PublicResponse<T>> {
  let response: Response
  try {
    response = await fetch(`/api/catalog/${path}`, {
      method: 'GET',
      credentials: 'omit',
      cache: 'no-store',
      signal,
    })
  } catch (err) {
    if (signal?.aborted) throw err
    throw new CatalogError('unavailable')
  }
  if (!response.ok)
    throw new CatalogError(
      response.status === 404
        ? 'not_found'
        : response.status === 503
          ? 'unavailable'
          : 'request_failed',
      response.status,
    )
  let body: unknown
  try {
    body = await response.json()
  } catch {
    throw new CatalogError('invalid_response', response.status)
  }
  const receivedAt = performance.now()
  if (
    !object(body) ||
    !object(body.meta) ||
    !validInstant(body.meta.server_time) ||
    !check(body.data)
  )
    throw new CatalogError('invalid_response', response.status)
  return { data: body.data, serverTime: body.meta.server_time, receivedAt }
}
export const publicCatalog = {
  categories: (signal?: AbortSignal) =>
    read('categories', list(category), signal),
  events: (signal?: AbortSignal) => read('events', list(event), signal),
  event: (slug: string, signal?: AbortSignal) =>
    read(`events/${encodeURIComponent(slug)}`, event, signal),
  lots: (signal?: AbortSignal) => read('lots', list(lot), signal),
  lot: (slug: string, signal?: AbortSignal) =>
    read(`lots/${encodeURIComponent(slug)}`, lot, signal),
}

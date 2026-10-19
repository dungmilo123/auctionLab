export type Category = { id: string; slug: string; name_en: string; name_vi: string; sort_order: number }
export type Image = { path: string; alt: string }
export type Specification = { name: string; value: string }
export type Fulfillment = { shipping_enabled: boolean; pickup_enabled: boolean; shipping_fee_minor: number | null; shipping_instructions: string; pickup_instructions: string }
export type LotContent = {
  slug: string; title: string; category_id: string | null; description: string; brand: string | null; era: string | null; story: string | null; condition: string;
  images: Image[]; specifications: Specification[]; fulfillment: Fulfillment; starting_price_minor: number | null; bid_increment_minor: number | null; currency: 'USD'; admin_notes: string;
}
export type Lot = LotContent & { id: string; state: 'draft' | 'published'; version: number; created_at: number; updated_at: number; demo: boolean; event_id: string | null }
export type EventContent = { slug: string; title: string; theme: string; description: string; cover_path: string; cover_alt: string; opens_at: number | null; first_closes_at: number | null; close_interval_ms: number | null; admin_notes: string }
export type Slot = { lot_id: string; position: number; opens_at: number; closes_at: number }
export type AuctionEvent = EventContent & { id: string; state: 'draft' | 'published'; version: number; created_at: number; updated_at: number; demo: boolean; lot_ids: string[]; schedule: Slot[] }
export type PublicLot = Omit<LotContent, 'admin_notes'> & { id: string; event_id: string; event_slug: string; position: number; opens_at: number | null; closes_at: number | null; state: string; demo: boolean }
export type PublicEvent = Omit<EventContent, 'admin_notes'> & { id: string; state: string; demo: boolean; planned_end: number | null; lots: PublicLot[] }
export class CatalogError extends Error {
  code: string
  status: number
  fields: Record<string, string>
  constructor(code: string, status = 0, fields: Record<string, string> = {}) { super(code); this.code = code; this.status = status; this.fields = fields }
}
export async function catalogRequest<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const mutation = method !== 'GET'
  let response: Response
  try {
    response = await fetch(`/api/admin/catalog/${path}`, { method, credentials: 'same-origin', cache: 'no-store',
      headers: mutation ? { 'Content-Type': 'application/json', 'X-AuctionLab-Admin': '1' } : {},
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    })
  } catch { throw new CatalogError(mutation ? 'commit_unknown' : 'unavailable') }
  let envelope: { data?: T; error?: { code?: string; fields?: Record<string, string> } }
  try { envelope = await response.json() } catch { throw new CatalogError(mutation ? 'commit_unknown' : 'unavailable', response.status) }
  if (!response.ok) throw new CatalogError(envelope.error?.code ?? 'storage_error', response.status, envelope.error?.fields)
  if (!('data' in envelope)) throw new CatalogError(mutation ? 'commit_unknown' : 'unavailable', response.status)
  return envelope.data as T
}
const idPath = (kind: string, id: string) => `${kind}/${encodeURIComponent(id)}`
export const catalogAPI = {
  categories: () => catalogRequest<Category[]>('categories'), lots: () => catalogRequest<Lot[]>('lots'), events: () => catalogRequest<AuctionEvent[]>('events'),
  lot: (id: string) => catalogRequest<Lot>(idPath('lots', id)), event: (id: string) => catalogRequest<AuctionEvent>(idPath('events', id)),
  createLot: (title: string) => catalogRequest<Lot>('lots', 'POST', { title }), createEvent: (title: string) => catalogRequest<AuctionEvent>('events', 'POST', { title }),
  saveLot: (lot: Lot, content: LotContent) => catalogRequest<Lot>(idPath('lots', lot.id), 'PUT', { ...content, version: lot.version }),
  saveEvent: (event: AuctionEvent, content: EventContent) => catalogRequest<AuctionEvent>(idPath('events', event.id), 'PUT', { ...content, version: event.version }),
  members: (event: AuctionEvent, ids: string[]) => catalogRequest<AuctionEvent>(`${idPath('events', event.id)}/lots`, 'PUT', { version: event.version, lot_ids: ids }),
  preview: (id: string) => catalogRequest<PublicEvent>(`${idPath('events', id)}/preview`),
  publish: (event: AuctionEvent) => catalogRequest<PublicEvent>(`${idPath('events', event.id)}/publish`, 'POST', { version: event.version }),
}
export function lotContent(l: Lot): LotContent { return { slug: l.slug, title: l.title, category_id: l.category_id, description: l.description, brand: l.brand, era: l.era, story: l.story, condition: l.condition, images: l.images, specifications: l.specifications, fulfillment: l.fulfillment, starting_price_minor: l.starting_price_minor, bid_increment_minor: l.bid_increment_minor, currency: l.currency, admin_notes: l.admin_notes } }
export function eventContent(e: AuctionEvent): EventContent { return { slug: e.slug, title: e.title, theme: e.theme, description: e.description, cover_path: e.cover_path, cover_alt: e.cover_alt, opens_at: e.opens_at, first_closes_at: e.first_closes_at, close_interval_ms: e.close_interval_ms, admin_notes: e.admin_notes } }

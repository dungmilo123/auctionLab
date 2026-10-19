import { AccountError } from './account'
import { validInstant, type PublicResponse } from './public-catalog'
import { MAX_CATALOG_MONEY as MAX } from '@/lib/catalog-money'
export type AuctionBid = { sequence: number; amount_minor: number; accepted_at: number; alias: string }
export type OwnBid = Omit<AuctionBid, 'alias'> & { request_id: string }
export type AuctionState = 'pending' | 'scheduled' | 'opening' | 'open' | 'closing' | 'sold' | 'unsold'
export type AuctionSnapshot = {
  lot_id: string; revision: number; auction_state: AuctionState; opens_at: number; closes_at: number; currency: 'USD'
  starting_price_minor: number; bid_increment_minor: number; current_price_minor: number | null; next_minimum_minor: number | null
  amount_limit_reached: boolean; bid_count: number; leader: AuctionBid | null; history: AuctionBid[]; history_has_more: boolean
  result: { outcome: 'sold' | 'unsold'; closes_at: number; recorded_at: number; winner: AuctionBid | null } | null
}
export type ParticipationStatus = 'not_participating' | 'leading' | 'outbid' | 'closing' | 'won' | 'lost'
export type Participation = { lot_id: string; slug: string; title: string; status: ParticipationStatus; latest: OwnBid | null; bids: OwnBid[]; current_price_minor: number | null; revision: number; checkout_available: false }
export type Intent = { lot_id: string; amount_minor: number; request_id: string }
export type Receipt = { lot_id: string; bid: OwnBid; replay: boolean }
export type AuctionResponse = PublicResponse<AuctionSnapshot>
const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
const integer = (v: unknown, min = 0): v is number => Number.isSafeInteger(v) && (v as number) >= min
const money = (v: unknown): v is number => integer(v) && v <= MAX
const maybeMoney = (v: unknown) => v === null || money(v)
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v)
function isBid(v: unknown): v is AuctionBid { return object(v) && integer(v.sequence, 1) && money(v.amount_minor) && validInstant(v.accepted_at) && typeof v.alias === 'string' && /^Bidder [1-9]\d*$/.test(v.alias) }
function isOwn(v: unknown): v is OwnBid { return object(v) && integer(v.sequence, 1) && money(v.amount_minor) && validInstant(v.accepted_at) && uuid(v.request_id) }
export function isAuctionSnapshot(v: unknown): v is AuctionSnapshot {
  if (!object(v) || typeof v.lot_id !== 'string' || !v.lot_id || !integer(v.revision) || !['pending','scheduled','opening','open','closing','sold','unsold'].includes(v.auction_state as string) || !validInstant(v.opens_at) || !validInstant(v.closes_at) || v.closes_at <= v.opens_at || v.currency !== 'USD' || !money(v.starting_price_minor) || !money(v.bid_increment_minor) || v.bid_increment_minor < 1 || !maybeMoney(v.current_price_minor) || !maybeMoney(v.next_minimum_minor) || typeof v.amount_limit_reached !== 'boolean' || !integer(v.bid_count) || !Array.isArray(v.history) || !v.history.every(isBid) || v.history.length > 50 || typeof v.history_has_more !== 'boolean' || !(v.leader === null || isBid(v.leader))) return false
  const noBids = v.bid_count === 0
  if (noBids !== (v.leader === null) || noBids !== (v.current_price_minor === null) || v.history.length !== Math.min(v.bid_count, 50) || v.history_has_more !== (v.bid_count > 50)) return false
  if (v.leader && (v.leader.sequence !== v.bid_count || v.leader.amount_minor !== v.current_price_minor)) return false
  if (!v.history.every((b, i) => b.sequence === (v.bid_count as number) - i && b.accepted_at >= (v.opens_at as number) && b.accepted_at < (v.closes_at as number))) return false
  if (v.leader && (v.history[0]?.amount_minor !== v.leader.amount_minor || v.history[0]?.alias !== v.leader.alias || v.history[0]?.accepted_at !== v.leader.accepted_at)) return false
  const expected = noBids ? v.starting_price_minor : (v.current_price_minor as number) + v.bid_increment_minor
  if (v.amount_limit_reached !== (expected > MAX) || v.next_minimum_minor !== (expected > MAX ? null : expected)) return false
  if ((v.auction_state === 'pending') !== (v.revision === 0) || v.auction_state === 'pending' && !noBids) return false
  const terminal = v.auction_state === 'sold' || v.auction_state === 'unsold'
  if (terminal) {
    if (!object(v.result) || v.result.outcome !== v.auction_state || v.result.closes_at !== v.closes_at || !validInstant(v.result.recorded_at) || v.result.recorded_at < v.closes_at) return false
    if (v.auction_state === 'sold') { if (!isBid(v.result.winner) || !v.leader || v.result.winner.sequence !== v.leader.sequence || v.result.winner.amount_minor !== v.current_price_minor || v.result.winner.alias !== v.leader.alias) return false }
    else if (!noBids || v.result.winner !== null) return false
  } else if (v.result !== null) return false
  return true
}
export function parseAuctionResponse(v: unknown): AuctionResponse {
  if (!object(v) || !isAuctionSnapshot(v.data) || !object(v.meta) || !validInstant(v.meta.server_time)) throw new AccountError('invalid_response')
  return { data: v.data, serverTime: v.meta.server_time, receivedAt: performance.now() }
}
export function mergeAuction(current: AuctionResponse | null, next: AuctionResponse): AuctionResponse {
  if (!current || current.data.lot_id !== next.data.lot_id) return next
  if (next.data.revision < current.data.revision || next.data.revision === current.data.revision && next.serverTime < current.serverTime) return current
  return next
}
export function isParticipation(v: unknown): v is Participation {
  return object(v) && ['lot_id','slug','title'].every(k => typeof v[k] === 'string') && ['not_participating','leading','outbid','closing','won','lost'].includes(v.status as string) && integer(v.revision) && maybeMoney(v.current_price_minor) && v.checkout_available === false && (v.latest === null || isOwn(v.latest)) && Array.isArray(v.bids) && v.bids.every(isOwn) && (v.status === 'not_participating' ? v.latest === null && v.bids.length === 0 : v.latest !== null && v.bids.length > 0)
}
export class AuctionError extends AccountError { snapshot?: AuctionResponse }
async function request(path: string, signal?: AbortSignal, intent?: Intent): Promise<Record<string, unknown>> {
  let response: Response
  try { response = await fetch(path, { method: intent ? 'POST' : 'GET', credentials: 'same-origin', cache: 'no-store', signal: signal ? AbortSignal.any([signal,AbortSignal.timeout(5000)]) : AbortSignal.timeout(5000),
    ...(intent ? { headers: { 'Content-Type': 'application/json', 'X-AuctionLab-Account': '1' }, body: JSON.stringify(intent) } : {}) }) }
  catch (e) { if (signal?.aborted) throw e; throw new AuctionError('unavailable') }
  let b: unknown
  try { b = await response.json() } catch { throw new AuctionError('invalid_response', response.status) }
  if (!response.ok) {
    const error = new AuctionError(object(b) && object(b.error) && typeof b.error.code === 'string' ? b.error.code : 'unavailable', response.status)
    if (object(b) && object(b.data) && b.data.snapshot) { try { error.snapshot = parseAuctionResponse({ data: b.data.snapshot, meta: b.meta }) } catch { /* Invalid snapshots cannot enable bidding. */ } }
    throw error
  }
  if (!object(b)) throw new AuctionError('invalid_response', response.status)
  return b
}
const lotPath = (id: string) => `/api/auctions/lots/${encodeURIComponent(id)}`
export const auctionAPI = {
  streamURL: (id: string) => `${lotPath(id)}/stream`,
  snapshot: async (id: string, signal?: AbortSignal) => parseAuctionResponse(await request(lotPath(id), signal)),
  history: async (id: string, before: number, signal?: AbortSignal): Promise<{ bids: AuctionBid[]; has_more: boolean; before_sequence: number | null }> => {
    const b = await request(`${lotPath(id)}/bids?before_sequence=${before}`, signal); const d = b.data
    if (!object(d) || !Array.isArray(d.bids) || d.bids.length > 50 || !d.bids.every(isBid) || !d.bids.every((v, i, all) => v.sequence < before && (i === 0 || v.sequence < all[i-1]!.sequence)) || typeof d.has_more !== 'boolean' || !(d.before_sequence === null || integer(d.before_sequence,1))) throw new AuctionError('invalid_response')
    return d as { bids: AuctionBid[]; has_more: boolean; before_sequence: number | null }
  },
  participation: async (id: string, signal?: AbortSignal): Promise<Participation> => { const b = await request(`/api/account/bids/${encodeURIComponent(id)}`, signal); if (!isParticipation(b.data) || b.data.lot_id !== id) throw new AuctionError('invalid_response'); return b.data },
  list: async (signal?: AbortSignal): Promise<Participation[]> => { const b = await request('/api/account/bids', signal); if (!Array.isArray(b.data) || !b.data.every(isParticipation) || new Set(b.data.map(p=>p.lot_id)).size!==b.data.length) throw new AuctionError('invalid_response'); return b.data },
  submit: async (intent: Intent, signal?: AbortSignal): Promise<{ receipt: Receipt; snapshot: AuctionResponse | null }> => {
    const b = await request('/api/account/bids', signal, intent); const d = b.data
    if (!object(d) || !object(d.receipt) || d.receipt.lot_id !== intent.lot_id || typeof d.receipt.replay !== 'boolean' || !isOwn(d.receipt.bid) || d.receipt.bid.amount_minor !== intent.amount_minor || d.receipt.bid.request_id !== intent.request_id) throw new AuctionError('invalid_response')
    let snapshot: AuctionResponse | null = null
    if (d.snapshot !== null) snapshot = parseAuctionResponse({ data: d.snapshot, meta: b.meta })
    return { receipt: d.receipt as Receipt, snapshot }
  },
}

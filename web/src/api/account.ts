import type { PublicLot } from './catalog'
import { isPublicLot, validInstant } from './public-catalog'
export type Profile = { id: string; email: string; display_name: string; role: 'bidder' | 'admin'; demo: boolean }
export class AccountError extends Error {
  code: string; status: number; fields: Record<string, string>
  constructor(code: string, status = 0, fields: Record<string, string> = {}) { super(code); this.code = code; this.status = status; this.fields = fields }
}
const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
export function isProfile(v: unknown): v is Profile {
  return object(v) && ['id', 'email', 'display_name'].every(k => typeof v[k] === 'string') && (v.role === 'bidder' || v.role === 'admin') && typeof v.demo === 'boolean'
}
async function request(path: string, method = 'GET', data?: unknown, signal?: AbortSignal): Promise<Record<string, unknown>> {
  let response: Response
  try { response = await fetch(path, { method, credentials: 'same-origin', cache: 'no-store', signal,
    ...(data === undefined ? {} : { headers: { 'Content-Type': 'application/json', 'X-AuctionLab-Account': '1' }, body: JSON.stringify(data) }) }) }
  catch (e) { if (signal?.aborted) throw e; throw new AccountError('unavailable') }
  let body: unknown
  try { body = await response.json() } catch { throw new AccountError(response.ok ? 'invalid_response' : 'unavailable', response.status) }
  if (!response.ok) {
    const error = object(body) && object(body.error) ? body.error : {}
    const fields: Record<string, string> = {}
    if (object(error.fields)) for (const [k, v] of Object.entries(error.fields)) if (typeof v === 'string') fields[k] = v
    throw new AccountError(typeof error.code === 'string' ? error.code : 'unavailable', response.status, fields)
  }
  if (!object(body)) throw new AccountError('invalid_response', response.status)
  return body
}
async function profile(path: string, method = 'GET', data?: unknown, signal?: AbortSignal): Promise<Profile> {
  const body = await request(path, method, data, signal)
  if (!isProfile(body.data)) throw new AccountError('invalid_response')
  const { id, email, display_name, role, demo } = body.data
  return { id, email, display_name, role, demo }
}
export const accountAPI = {
  profile: (signal?: AbortSignal) => profile('/api/account', 'GET', undefined, signal),
  login: (data: { email: string; password: string }) => profile('/api/auth/login', 'POST', data),
  register: (data: { email: string; password: string; display_name: string }) => profile('/api/auth/register', 'POST', data),
  update: (display_name: string) => profile('/api/account', 'PATCH', { display_name }),
  logout: async () => { const b = await request('/api/auth/logout', 'POST', {}); if (!object(b.data) || b.data.signed_out !== true) throw new AccountError('invalid_response') },
  membership: async (id: string, saved?: boolean, signal?: AbortSignal) => {
    const b = await request(`/api/account/watchlist/${encodeURIComponent(id)}`, saved === undefined ? 'GET' : saved ? 'PUT' : 'DELETE', saved === undefined ? undefined : {}, signal)
    if (!object(b.data) || typeof b.data.saved !== 'boolean') throw new AccountError('invalid_response')
    return b.data.saved
  },
  watchlist: async (signal?: AbortSignal): Promise<{ lots: PublicLot[]; serverTime: number; receivedAt: number }> => {
    const b = await request('/api/account/watchlist', 'GET', undefined, signal)
    if (!Array.isArray(b.data) || !b.data.every(isPublicLot) || !object(b.meta) || !validInstant(b.meta.server_time)) throw new AccountError('invalid_response')
    return { lots: b.data, serverTime: b.meta.server_time, receivedAt: performance.now() }
  },
}

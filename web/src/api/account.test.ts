import { afterEach, expect, it, vi } from 'vitest'
import { accountAPI, AccountError } from './account'
afterEach(() => vi.unstubAllGlobals())
const p = { id: 'a', email: 'a@example.test', display_name: 'Name', role: 'bidder', demo: false }
it('uses same-origin no-store requests and explicit JSON mutation headers', async () => {
  const fetcher = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: p })))
  vi.stubGlobal('fetch', fetcher)
  await accountAPI.login({ email: 'a@example.test', password: 'fixture password' })
  const [path, init] = fetcher.mock.calls[0]!
  expect(path).toBe('/api/auth/login'); expect(init).toMatchObject({ credentials: 'same-origin', cache: 'no-store', method: 'POST', headers: { 'X-AuctionLab-Account': '1', 'Content-Type': 'application/json' } })
  await accountAPI.profile(); expect(fetcher.mock.calls[1]![1].body).toBeUndefined()
})
it('strips unrecognized profile fields and rejects malformed success data', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ data: { ...p, verifier: 'private' } }))).mockResolvedValueOnce(new Response(JSON.stringify({ data: { ...p, role: 'root' } }))))
  expect(await accountAPI.profile()).toEqual(p); await expect(accountAPI.profile()).rejects.toMatchObject({ code: 'invalid_response' })
})
it('keeps validation, duplicate, invalid credentials, throttle and storage failures distinct', async () => {
  for (const [status, code] of [[422, 'validation'], [409, 'duplicate_email'], [401, 'invalid_credentials'], [429, 'throttled'], [503, 'commit_unknown']] as const) {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code, fields: { email: 'invalid' } } }), { status })))
    await expect(accountAPI.login({ email: '', password: '' })).rejects.toMatchObject({ status, code, fields: { email: 'invalid' } })
  }
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network'))); await expect(accountAPI.profile()).rejects.toBeInstanceOf(AccountError)
})
it('validates membership and list envelopes without inventing empty data', async () => {
  const f = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ data: { saved: true } }))).mockResolvedValueOnce(new Response(JSON.stringify({ data: [], meta: { server_time: 1000 } }))).mockResolvedValueOnce(new Response(JSON.stringify({ data: [] })))
  vi.stubGlobal('fetch', f)
  expect(await accountAPI.membership('lot/id', true)).toBe(true); expect(f.mock.calls[0]![0]).toBe('/api/account/watchlist/lot%2Fid'); expect(f.mock.calls[0]![1].body).toBe('{}')
  expect((await accountAPI.watchlist()).lots).toEqual([]); await expect(accountAPI.watchlist()).rejects.toMatchObject({ code: 'invalid_response' })
})

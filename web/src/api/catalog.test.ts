import { afterEach, describe, expect, it, vi } from 'vitest'
import { catalogRequest, CatalogError } from './catalog'
afterEach(() => vi.unstubAllGlobals())
describe('catalog HTTP boundary', () => {
  it('sends cookie credentials and explicit mutation headers without any key', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { id: 'saved' } }), { status: 201 }))
    vi.stubGlobal('fetch', fetch)
    expect(await catalogRequest('lots', 'POST', { title: 'Camera' })).toEqual({ id: 'saved' })
    expect(fetch).toHaveBeenCalledWith('/api/admin/catalog/lots', expect.objectContaining({ credentials: 'same-origin', cache: 'no-store', headers: { 'Content-Type': 'application/json', 'X-AuctionLab-Admin': '1' }, body: '{"title":"Camera"}' }))
  })
  it.each([[401, 'access_required'], [409, 'stale_version'], [422, 'validation'], [503, 'commit_unknown']])('maps status %s to stable feedback', async (status, code) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code, fields: { title: 'required' } } }), { status: Number(status) })))
    await expect(catalogRequest('lots')).rejects.toMatchObject({ code, status, fields: { title: 'required' } })
  })
  it('distinguishes unavailable reads and uncertain mutation transport failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    await expect(catalogRequest('lots')).rejects.toEqual(new CatalogError('unavailable'))
    await expect(catalogRequest('lots', 'POST', { title: 'Retain input' })).rejects.toEqual(new CatalogError('commit_unknown'))
  })
  it('does not invent success from HTML or missing envelopes', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html/>', { status: 200 })))
    await expect(catalogRequest('lots')).rejects.toMatchObject({ code: 'unavailable' })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 200 })))
    await expect(catalogRequest('lots', 'PUT', {})).rejects.toMatchObject({ code: 'commit_unknown' })
  })
})

import { afterEach, expect, it, vi } from 'vitest'
import { publicCatalog } from './public-catalog'
afterEach(() => vi.unstubAllGlobals())
const response = (data: unknown, server_time: unknown = 1000) =>
  new Response(JSON.stringify({ data, meta: { server_time } }))
it('reads all empty public collections anonymously with cancellation', async () => {
  const fetch = vi.fn().mockImplementation(() => Promise.resolve(response([])))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  for (const read of [
    publicCatalog.categories,
    publicCatalog.events,
    publicCatalog.lots,
  ])
    expect((await read(signal)).data).toEqual([])
  for (const [path, options] of fetch.mock.calls) {
    expect(path).toMatch(/^\/api\/catalog\//)
    expect(options).toEqual({
      method: 'GET',
      credentials: 'omit',
      cache: 'no-store',
      signal,
    })
  }
})
it('encodes detail slugs and rejects malformed detail records', async () => {
  const fetch = vi
    .fn()
    .mockImplementation(() => Promise.resolve(response({ slug: 'bad' })))
  vi.stubGlobal('fetch', fetch)
  await expect(publicCatalog.lot('a/b ?')).rejects.toMatchObject({
    code: 'invalid_response',
  })
  expect(fetch.mock.calls[0]![0]).toBe('/api/catalog/lots/a%2Fb%20%3F')
  await expect(publicCatalog.event('a/b')).rejects.toMatchObject({
    code: 'invalid_response',
  })
})
it.each([
  [404, 'not_found'],
  [503, 'unavailable'],
  [500, 'request_failed'],
])('sanitizes HTTP %s', async (status, code) => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response('private backend details', { status: Number(status) }),
      ),
  )
  await expect(publicCatalog.lots()).rejects.toMatchObject({ code, status })
})
it.each([
  '<html/>',
  '{}',
  'null',
  '{"data":[],"meta":{"server_time":-1}}',
  '{"data":[],"meta":{"server_time":1.5}}',
  '{"data":{},"meta":{"server_time":1000}}',
])('rejects invalid success %s', async (raw) => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(raw)))
  await expect(publicCatalog.events()).rejects.toMatchObject({
    code: 'invalid_response',
  })
})
it('sanitizes transport failures and preserves cancellation', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockRejectedValue(new Error('private transport error')),
  )
  await expect(publicCatalog.lots()).rejects.toMatchObject({
    code: 'unavailable',
  })
  const controller = new AbortController()
  controller.abort()
  await expect(publicCatalog.lots(controller.signal)).rejects.toThrow(
    'private transport error',
  )
})

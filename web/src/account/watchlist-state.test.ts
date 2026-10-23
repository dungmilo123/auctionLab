import { expect, it, vi } from 'vitest'
import { accountAPI, AccountError } from '@/api/account'
import type { PublicLot } from '@/api/catalog'
import { WatchlistStore } from './watchlist-state'
const lot = { id: 'lot' } as PublicLot
const list = (lots: PublicLot[] = []) => ({ lots, serverTime: 10, receivedAt: 1 })
const deferred = <T>() => { let resolve!: (v: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }
it('shares one list read, waits for confirmation and suppresses duplicate writes', async () => {
  const read = deferred<ReturnType<typeof list>>(), write = deferred<boolean>()
  const api = { ...accountAPI, watchlist: vi.fn(() => read.promise), membership: vi.fn(() => write.promise) }
  const s = new WatchlistStore(api), listener = vi.fn();s.subscribe(listener);s.subscribe(vi.fn())
  const load = s.load();void s.load();expect(api.watchlist).toHaveBeenCalledTimes(1)
  expect(s.get().status).toBe('loading');expect(s.get().value).toBeNull()
  read.resolve(list());await load
  const save = s.toggle(lot);void s.toggle(lot)
  expect(api.membership).toHaveBeenCalledTimes(1);expect(s.get().value!.lots).toEqual([])
  expect(s.get().pending.has('lot')).toBe(true)
  write.resolve(true);await save;expect(s.get().value!.lots).toEqual([lot]);expect(listener).toHaveBeenCalled()
})
it('makes uncertain membership unknown and only recovers with a list read', async () => {
  const api = { ...accountAPI, watchlist: vi.fn().mockResolvedValue(list()), membership: vi.fn().mockRejectedValue(new AccountError('unavailable')) }
  const s = new WatchlistStore(api);await s.load();await s.toggle(lot)
  expect(s.get().status).toBe('error');expect(s.get().value).toBeNull()
  await s.toggle(lot);expect(api.membership).toHaveBeenCalledTimes(1)
  api.watchlist.mockResolvedValue(list([lot]));await s.load();expect(s.get().value!.lots).toEqual([lot]);expect(api.membership).toHaveBeenCalledTimes(1)
})
it('clears synchronously and suppresses late reads and writes after identity invalidation', async () => {
  const read = deferred<ReturnType<typeof list>>(), write = deferred<boolean>()
  const api = { ...accountAPI, watchlist: vi.fn(() => read.promise), membership: vi.fn(() => write.promise) }
  const old = new WatchlistStore(api), load = old.load();old.clear();read.resolve(list([lot]));await load
  expect(old.get().value).toBeNull()
  await old.load();const save = old.toggle(lot);old.clear();expect(old.get().value).toBeNull();write.resolve(false);await save
  expect(old.get().value).toBeNull();const fresh = new WatchlistStore(api);expect(fresh.get().value).toBeNull()
})
it('clears other pending responses on failure and signs out on 401', async () => {
  const late = deferred<boolean>(), unauthorized = vi.fn()
  const api = { ...accountAPI, watchlist: vi.fn().mockResolvedValue(list()), membership: vi.fn().mockImplementationOnce(() => late.promise).mockRejectedValue(new AccountError('signed_out',401)) }
  const s = new WatchlistStore(api, unauthorized);await s.load()
  const a=s.toggle(lot);await s.toggle({id:'another'} as PublicLot);late.resolve(true);await a
  expect(unauthorized).toHaveBeenCalledTimes(1);expect(s.get().value).toBeNull();expect(s.get().status).toBe('error')
})

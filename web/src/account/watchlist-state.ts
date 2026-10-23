import { accountAPI, AccountError } from '@/api/account'
import type { PublicLot } from '@/api/catalog'
type List = Awaited<ReturnType<typeof accountAPI.watchlist>>
export type WatchState = { status: 'loading' | 'ready' | 'error'; value: List | null; pending: ReadonlySet<string>; error?: AccountError }
// One in-memory list per validated identity generation. No persistence or polling.
export class WatchlistStore {
  private state: WatchState = { status: 'loading', value: null, pending: new Set() }
  private listeners = new Set<() => void>()
  private generation = 0
  private controllers = new Set<AbortController>()
  private reading = false
  private api: typeof accountAPI
  private unauthorized: () => void
  constructor(api = accountAPI, unauthorized = () => {}) { this.api = api; this.unauthorized = unauthorized }
  get = () => this.state
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn) } }
  private set(value: WatchState) { this.state = value; this.listeners.forEach(fn => fn()) }
  clear = () => {
    this.generation++; this.controllers.forEach(c => c.abort()); this.controllers.clear(); this.reading = false
    this.set({ status: 'loading', value: null, pending: new Set() })
  }
  load = async () => {
    if (this.reading || this.state.pending.size) return
    this.reading = true
    const generation = this.generation, c = new AbortController(); this.controllers.add(c)
    this.set({ status: 'loading', value: null, pending: new Set() })
    try { const value = await this.api.watchlist(c.signal); if (generation === this.generation) this.set({ status: 'ready', value, pending: new Set() }) }
    catch (e) { if (generation === this.generation) this.fail(e) }
    finally { if (generation === this.generation) this.reading = false; this.controllers.delete(c) }
  }
  private fail(e: unknown) {
    const error = e instanceof AccountError ? e : new AccountError('unavailable')
    this.set({ status: 'error', value: null, pending: new Set(), error })
    if (error.status === 401) this.unauthorized()
  }
  toggle = async (lot: PublicLot) => {
    if (this.state.status !== 'ready' || !this.state.value || this.state.pending.has(lot.id)) return
    const generation = this.generation, saved = this.state.value.lots.some(l => l.id === lot.id)
    const c = new AbortController(); this.controllers.add(c)
    this.set({ ...this.state, pending: new Set([...this.state.pending, lot.id]) })
    try {
      const confirmed = await this.api.membership(lot.id, !saved, c.signal)
      if (generation !== this.generation || this.state.status !== 'ready' || !this.state.value) return
      const lots = this.state.value.lots.filter(l => l.id !== lot.id)
      if (confirmed) lots.push(lot)
      const pending = new Set(this.state.pending); pending.delete(lot.id)
      this.set({ status: 'ready', value: { ...this.state.value, lots }, pending })
    } catch (e) { if (generation === this.generation) { this.generation++; this.controllers.forEach(x => x.abort()); this.fail(e) } }
    finally { this.controllers.delete(c) }
  }
}

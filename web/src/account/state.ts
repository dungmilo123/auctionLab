import { AccountError, accountAPI, type Profile } from '@/api/account'
export type AccountState = { status: 'checking' } | { status: 'signed-out' } | { status: 'unavailable' } | { status: 'signed-in'; profile: Profile }
export class AccountStore {
  private state: AccountState = { status: 'checking' }
  private generation = 0
  private pending?: AbortController
  private listeners = new Set<() => void>()
  private load: typeof accountAPI.profile
  constructor(load = accountAPI.profile) { this.load = load }
  get = () => this.state
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn) } }
  private set(state: AccountState) { this.state = state; for (const f of this.listeners) f() }
  clear = () => { this.generation++; this.pending?.abort(); this.set({ status: 'checking' }) }
  signedOut = () => { this.clear(); this.set({ status: 'signed-out' }) }
  confirm = (profile: Profile, previous: AccountState) => { if (this.state === previous) this.set({ status: 'signed-in', profile }) }
  revalidate = async () => {
    this.clear(); const generation = this.generation; const controller = new AbortController(); this.pending = controller
    try { const profile = await this.load(controller.signal); if (generation === this.generation) this.set({ status: 'signed-in', profile }) }
    catch (error) { if (generation === this.generation) this.set({ status: error instanceof AccountError && error.status === 401 ? 'signed-out' : 'unavailable' }) }
  }
}

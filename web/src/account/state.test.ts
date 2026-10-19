import { expect, it } from 'vitest'
import { AccountStore } from './state'
import { AccountError, type Profile } from '@/api/account'
const p: Profile = { id: 'a', email: 'a@example.test', display_name: 'A', role: 'bidder', demo: false }
it('separates signed-out and unavailable and clears private identity before a recheck', async () => {
  let error: AccountError | undefined
  const s = new AccountStore(async () => { if (error) throw error; return p })
  await s.revalidate(); expect(s.get()).toEqual({ status: 'signed-in', profile: p })
  error = new AccountError('unavailable', 503)
  const check = s.revalidate(); expect(s.get()).toEqual({ status: 'checking' }); await check; expect(s.get().status).toBe('unavailable')
  error = new AccountError('access_required', 401); await s.revalidate(); expect(s.get().status).toBe('signed-out')
})
it('ignores late identity responses, aborts old reads and rejects obsolete mutation confirmations', async () => {
  const resolvers: ((p: Profile) => void)[] = []; const signals: AbortSignal[] = []
  const s = new AccountStore(signal => { signals.push(signal!); return new Promise(resolve => { resolvers.push(resolve) }) })
  const old = s.revalidate(); const current = s.revalidate(); expect(signals[0]!.aborted).toBe(true)
  resolvers[1]!({ ...p, id: 'b' }); await current; const state = s.get(); resolvers[0]!(p); await old; expect(s.get()).toBe(state)
  s.signedOut(); s.confirm(p, state); expect(s.get().status).toBe('signed-out')
})

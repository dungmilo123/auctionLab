import { useCallback, useEffect, useRef, useState } from 'react'
import { accountAPI, AccountError } from '@/api/account'
import { useAccount } from '@/account/AccountProvider'
import { useAccountCopy, accountErrorKey } from '@/lib/account-copy'
import { authURL } from '@/lib/account-return'
import { usePrivateRead } from './usePrivateRead'
function SignedInWatch({ id }: { id: string }) {
  const { a } = useAccountCopy(); const { store } = useAccount()
  const load = useCallback((signal: AbortSignal) => accountAPI.membership(id, undefined, signal), [id])
  const { state, setState, retry } = usePrivateRead(load); const [pending, setPending] = useState(false); const [message, setMessage] = useState<'saved' | 'removed' | null>(null)
  const alive = useRef(true); useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  async function mutate() {
    if (pending || state.status !== 'success') return; setPending(true); setMessage(null)
    try { const saved = await accountAPI.membership(id, !state.data); if (alive.current) { setState({ status: 'success', data: saved }); setMessage(saved ? 'saved' : 'removed') } }
    catch (e) { if (alive.current) { const error = e instanceof AccountError ? e : new AccountError('unavailable'); if (error.status === 401) store.signedOut(); else setState({ status: 'error', error }) } }
    finally { if (alive.current) setPending(false) }
  }
  return <div className="stack-small">
    {state.status === 'success' ? <button className="site-button site-button-outline" disabled={pending} onClick={() => void mutate()}>{a(pending ? 'pending' : state.data ? 'removeLot' : 'saveLot')}</button>
      : state.status === 'loading' ? <p role="status">{a('watchLoading')}</p>
        : <><p role="alert">{a(accountErrorKey(state.error.code))}</p><button className="site-button site-button-outline" onClick={() => { setMessage(null); retry() }}>{a('reload')}</button></>}
    <p role="status">{message ? a(message) : ''}</p>
  </div>
}
export default function WatchControl({ id, target }: { id: string; target: string }) {
  const { state, store } = useAccount(); const { a } = useAccountCopy()
  return <section className="empty-state stack-small" aria-label={a('watchlist')}>
    <h2>{a('watchlist')}</h2><p>{a('watchHelp')}</p>
    {state.status === 'checking' ? <p role="status">{a('checking')}</p> : state.status === 'signed-out' ? <a className="site-button site-button-outline" href={authURL('login', target)}>{a('loginSave')}</a>
      : state.status === 'unavailable' ? <><p role="status">{a('recovery')}</p><button className="site-button" onClick={() => void store.revalidate()}>{a('retry')}</button></>
        : <SignedInWatch key={`${state.profile.id}:${id}`} id={id} />}
  </section>
}

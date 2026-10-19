import { useEffect, useRef, useState } from 'react'
import { accountAPI, AccountError } from '@/api/account'
import { useAccount } from '@/account/AccountProvider'
import { useAccountCopy, accountErrorKey } from '@/lib/account-copy'
import { PublicLotCard } from '@/routes/storefront/DiscoveryPresentation'
import { usePrivateRead } from './usePrivateRead'
export default function Watchlist() {
  const { a } = useAccountCopy(); const { store } = useAccount()
  const { state, setState, retry } = usePrivateRead(accountAPI.watchlist)
  const [pending, setPending] = useState<string | null>(null); const [removed, setRemoved] = useState(false); const [clock, setClock] = useState(() => performance.now())
  const resultNode = useRef<HTMLParagraphElement>(null)
  const alive = useRef(true); useEffect(() => { alive.current = true; const timer = window.setInterval(() => setClock(performance.now()), 1000); return () => { alive.current = false; clearInterval(timer) } }, [])
  async function remove(id: string) {
    if (pending || state.status !== 'success') return; setPending(id); setRemoved(false)
    try { const saved = await accountAPI.membership(id, false); if (saved) throw new AccountError('invalid_response'); if (alive.current) { setState({ status: 'success', data: { ...state.data, lots: state.data.lots.filter(l => l.id !== id) } }); setRemoved(true); requestAnimationFrame(() => resultNode.current?.focus()) } }
    catch (e) { if (alive.current) { const error = e instanceof AccountError ? e : new AccountError('unavailable'); if (error.status === 401) store.signedOut(); else { setState({ status: 'error', error }); requestAnimationFrame(() => resultNode.current?.focus()) } } }
    finally { if (alive.current) setPending(null) }
  }
  if (state.status === 'loading') return <p role="status">{a('watchLoading')}</p>
  if (state.status === 'error') return <section className="empty-state stack"><h2>{a(state.error.code === 'unavailable' ? 'unavailable' : 'watchError')}</h2><p ref={resultNode} tabIndex={-1} role="alert">{a(accountErrorKey(state.error.code))}</p><button className="site-button" onClick={retry}>{a('reload')}</button></section>
  const { lots, serverTime, receivedAt } = state.data
  const now = serverTime + Math.max(0, clock - receivedAt)
  return <section className="stack"><p>{a('watchHelp')}</p><p className="numeric">{a(lots.length === 1 ? 'countOne' : 'count', { count: String(lots.length) })}</p><p ref={resultNode} tabIndex={-1} role="status">{removed ? a('removed') : ''}</p>
    {lots.length === 0 ? <div className="empty-state stack"><h2>{a('empty')}</h2><p>{a('emptyHelp')}</p><a className="site-button site-button-outline" href="/lots">{a('browse')}</a></div>
      : <div className="lot-grid">{lots.map(lot => <div className="stack-small" key={lot.id}><PublicLotCard lot={lot} categories={[]} now={now} showCategory={false} /><button className="site-button site-button-outline" disabled={pending !== null} onClick={() => void remove(lot.id)} aria-label={`${a('removeLot')}: ${lot.title}`}>{a(pending === lot.id ? 'pending' : 'removeLot')}</button></div>)}</div>}
  </section>
}

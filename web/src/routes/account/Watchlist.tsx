import { useEffect, useRef, useState } from 'react'
import { useWatchlist, useMembershipState } from '@/account/WatchlistProvider'
import type { WatchlistStore } from '@/account/watchlist-state'
import { useAccountCopy } from '@/lib/account-copy'
import { PublicLotCard } from '@/routes/storefront/DiscoveryPresentation'
function SavedLots({ store }: { store: WatchlistStore }) {
  const { a } = useAccountCopy(), state = useMembershipState(store)
  const [clock, setClock] = useState(() => performance.now()), result = useRef<HTMLParagraphElement>(null)
  const previous = useRef(state.value?.lots.length ?? 0)
  useEffect(() => { const timer = window.setInterval(() => setClock(performance.now()), 1000); return () => clearInterval(timer) }, [])
  useEffect(() => { const count = state.value?.lots.length; if (count !== undefined && count < previous.current) result.current?.focus(); previous.current = count ?? 0 }, [state.value])
  if (state.status === 'loading') return <p role="status">{a('watchLoading')}</p>
  if (state.status === 'error' || !state.value) return <section className="empty-state stack"><h2>{a('watchError')}</h2><p role="alert">{a('recovery')}</p><button className="site-button" onClick={() => void store.load()}>{a('reload')}</button></section>
  const { lots, serverTime, receivedAt } = state.value
  return <section className="stack"><p ref={result} tabIndex={-1} role="status">{a(lots.length === 1 ? 'countOne' : 'count', { count: String(lots.length) })}</p>{lots.length ? <div className="lot-grid">{lots.map(lot => <PublicLotCard key={lot.id} lot={lot} categories={[]} now={serverTime + Math.max(0, clock - receivedAt)} showCategory={false} />)}</div> : <div className="empty-state stack"><h2>{a('empty')}</h2><p>{a('emptyHelp')}</p><a className="site-button site-button-outline" href="/lots">{a('browse')}</a></div>}</section>
}
export default function Watchlist() { const store = useWatchlist(); return store ? <SavedLots store={store} /> : null }

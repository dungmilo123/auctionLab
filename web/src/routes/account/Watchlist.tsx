import { useEffect, useRef, useState } from 'react'
import { useWatchlist, useMembershipState } from '@/account/WatchlistProvider'
import type { WatchlistStore } from '@/account/watchlist-state'
import { useAccountCopy } from '@/lib/account-copy'
import { PublicLotCard } from '@/routes/storefront/DiscoveryPresentation'
import { Button } from '@/components/ui/button'
function SavedLots({ store }: { store: WatchlistStore }) {
  const { a } = useAccountCopy(), state = useMembershipState(store)
  const [clock, setClock] = useState(() => performance.now()), result = useRef<HTMLParagraphElement>(null)
  const previous = useRef(state.value?.lots.length ?? 0)
  useEffect(() => { const timer = window.setInterval(() => setClock(performance.now()), 1000); return () => clearInterval(timer) }, [])
  useEffect(() => { const count = state.value?.lots.length; if (count !== undefined && count < previous.current) result.current?.focus(); previous.current = count ?? 0 }, [state.value])
  if (state.status === 'loading') return <p role="status">{a('watchLoading')}</p>
  if (state.status === 'error' || !state.value) return <section className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8"><h2>{a('watchError')}</h2><p role="alert" className="text-muted-foreground">{a('recovery')}</p><Button variant="outline" className="self-start" onClick={() => void store.load()}>{a('reload')}</Button></section>
  const { lots, serverTime, receivedAt } = state.value
  return <section className="flex flex-col gap-6"><p ref={result} tabIndex={-1} role="status" className="mono-label">{a(lots.length === 1 ? 'countOne' : 'count', { count: String(lots.length) })}</p>{lots.length ? <div className="grid grid-cols-1 gap-7 sm:grid-cols-2 lg:grid-cols-3 [&>*:nth-child(3n+1)]:rotate-[-0.5deg] [&>*:nth-child(3n+2)]:rotate-[0.45deg] [&>*:nth-child(3n)]:rotate-[-0.2deg]">{lots.map(lot => <PublicLotCard key={lot.id} lot={lot} categories={[]} now={serverTime + Math.max(0, clock - receivedAt)} showCategory={false} />)}</div> : <div className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8"><h2>{a('empty')}</h2><p className="text-muted-foreground">{a('emptyHelp')}</p><Button asChild variant="outline" className="self-start"><a href="/lots">{a('browse')}</a></Button></div>}</section>
}
export default function Watchlist() { const store = useWatchlist(); return store ? <SavedLots store={store} /> : null }

import { Heart, CircleHelp } from 'lucide-react'
import { cn } from 'cn'
import { Icon } from '@/components/site/Icon'
import { Button } from '@/components/ui/button'
import { useAccount } from '@/account/AccountProvider'
import { useWatchlist, useMembershipState } from '@/account/WatchlistProvider'
import type { WatchlistStore } from '@/account/watchlist-state'
import type { PublicLot } from '@/api/catalog'
import { useAccountCopy } from '@/lib/account-copy'
import { authURL } from '@/lib/account-return'
function ConfirmedControl({ store, lot, compact }: { store: WatchlistStore; lot: PublicLot; compact: boolean }) {
  const state = useMembershipState(store), { a } = useAccountCopy()
  const known = state.status === 'ready', saved = known && !!state.value?.lots.some(l => l.id === lot.id), pending = state.pending.has(lot.id)
  const label = a(state.status === 'error' ? 'reload' : !known ? 'watchLoading' : pending ? 'pending' : saved ? 'removeLot' : 'saveLot')
  return <Button type="button" variant="outline" className={cn(compact && 'size-[38px] min-h-0 min-w-0 rounded-full bg-paper p-0', saved && 'motion-safe:animate-[confirmed-feedback_150ms_ease-out]')} aria-label={`${label}: ${lot.title}`} aria-pressed={known ? saved : undefined} disabled={state.status === 'loading' || pending} onClick={() => { if (state.status === 'error') void store.load(); else void store.toggle(lot) }}>
    <Icon icon={known ? Heart : CircleHelp} />{!compact && label}
  </Button>
}
export default function WatchControl({ lot, target, compact = false }: { lot: PublicLot; target: string; compact?: boolean }) {
  const { state, store: account } = useAccount(), store = useWatchlist(), { a } = useAccountCopy()
  const content = state.status === 'signed-in' && store ? <ConfirmedControl store={store} lot={lot} compact={compact} />
    : state.status === 'signed-out' ? <Button asChild variant="outline" className={cn(compact && 'size-[38px] min-h-0 min-w-0 rounded-full bg-paper p-0')}><a href={authURL('login', target)} aria-label={`${a('loginSave')}: ${lot.title}`}><Icon icon={Heart} />{!compact && a('loginSave')}</a></Button>
      : <Button type="button" variant="outline" className={cn(compact && 'size-[38px] min-h-0 min-w-0 rounded-full bg-paper p-0')} disabled={state.status === 'checking'} aria-label={`${a(state.status === 'checking' ? 'checking' : 'recovery')}: ${lot.title}`} onClick={() => void account.revalidate()}><Icon icon={CircleHelp} />{!compact && a(state.status === 'checking' ? 'checking' : 'retry')}</Button>
  return <div className={compact ? 'absolute top-2.5 right-2.5 z-[2]' : undefined}>{content}</div>
}

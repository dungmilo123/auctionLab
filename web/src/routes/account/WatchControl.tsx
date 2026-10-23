import { Heart, CircleHelp } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
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
  return <button type="button" className={`site-button site-button-outline ${saved ? 'save-confirmed' : ''}`} aria-label={`${label}: ${lot.title}`} aria-pressed={known ? saved : undefined} disabled={state.status === 'loading' || pending} onClick={() => { if (state.status === 'error') void store.load(); else void store.toggle(lot) }}>
    <Icon icon={known ? Heart : CircleHelp} />{!compact && label}
  </button>
}
export default function WatchControl({ lot, target, compact = false }: { lot: PublicLot; target: string; compact?: boolean }) {
  const { state, store: account } = useAccount(), store = useWatchlist(), { a } = useAccountCopy()
  const content = state.status === 'signed-in' && store ? <ConfirmedControl store={store} lot={lot} compact={compact} />
    : state.status === 'signed-out' ? <a className="site-button site-button-outline" href={authURL('login', target)} aria-label={`${a('loginSave')}: ${lot.title}`}><Icon icon={Heart} />{!compact && a('loginSave')}</a>
      : <button type="button" className="site-button site-button-outline" disabled={state.status === 'checking'} aria-label={`${a(state.status === 'checking' ? 'checking' : 'recovery')}: ${lot.title}`} onClick={() => void account.revalidate()}><Icon icon={CircleHelp} />{!compact && a(state.status === 'checking' ? 'checking' : 'retry')}</button>
  return <div className={compact ? 'card-save' : 'watch-action'}>{content}</div>
}

import { createContext, useContext, useEffect, useMemo, useSyncExternalStore, type ReactNode } from 'react'
import { useAccount } from './AccountProvider'
import { WatchlistStore } from './watchlist-state'
const Context = createContext<WatchlistStore | null>(null)
export function WatchlistProvider({ children }: { children: ReactNode }) {
  const { state, store: account } = useAccount()
  const identity = state.status === 'signed-in' ? state.profile.id : null
  const store = useMemo(() => identity ? new WatchlistStore(undefined, account.signedOut) : null, [identity, account])
  useEffect(() => {
    let active = true
    queueMicrotask(() => { if (active && store) void store.load() })
    return () => { active = false; store?.clear() }
  }, [store])
  return <Context.Provider value={store}>{children}</Context.Provider>
}
// oxlint-disable-next-line react/only-export-components
export function useWatchlist() { return useContext(Context) }
// oxlint-disable-next-line react/only-export-components
export function useMembershipState(store: WatchlistStore) { return useSyncExternalStore(store.subscribe, store.get, store.get) }

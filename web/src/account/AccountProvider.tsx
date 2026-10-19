import { createContext, useContext, useEffect, useState, useSyncExternalStore, type ReactNode } from 'react'
import { AccountStore } from './state'
const Context = createContext<AccountStore | null>(null)
export function AccountProvider({ children }: { children: ReactNode }) {
  const [store] = useState(() => new AccountStore())
  useEffect(() => {
    const recheck = () => { void store.revalidate() }
    recheck()
    window.addEventListener('pageshow', recheck)
    window.addEventListener('focus', recheck)
    window.addEventListener('pagehide', store.clear)
    return () => { store.clear(); window.removeEventListener('pageshow', recheck); window.removeEventListener('focus', recheck); window.removeEventListener('pagehide', store.clear) }
  }, [store])
  return <Context.Provider value={store}>{children}</Context.Provider>
}
// Shared context hook intentionally lives with its provider.
// oxlint-disable-next-line react/only-export-components
export function useAccount() {
  const store = useContext(Context)
  if (!store) throw new Error('AccountProvider required')
  const state = useSyncExternalStore(store.subscribe, store.get, store.get)
  return { state, store }
}

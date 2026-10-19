import type { ReactNode } from 'react'
import { useAccount } from '@/account/AccountProvider'
import { useAccountCopy } from '@/lib/account-copy'
import { authURL } from '@/lib/account-return'
export function AccountStatus({ children, target = '/account' }: { children: ReactNode; target?: string }) {
  const { state, store } = useAccount(); const { a } = useAccountCopy()
  if (state.status === 'signed-in') return children
  if (state.status === 'checking') return <p role="status">{a('checking')}</p>
  return <section className="empty-state stack"><h2>{a(state.status === 'signed-out' ? 'signedOut' : 'unavailable')}</h2>
    <p>{a(state.status === 'signed-out' ? 'loginRequired' : 'recovery')}</p>
    <div className="actions">{state.status === 'signed-out' ? <><a className="site-button site-button-primary" href={authURL('login', target)}>{a('login')}</a><a className="site-button site-button-outline" href={authURL('register', target)}>{a('register')}</a></> : <button className="site-button" onClick={() => void store.revalidate()}>{a('retry')}</button>}</div></section>
}

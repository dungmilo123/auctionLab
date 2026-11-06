import type { ReactNode } from 'react'
import { useAccount } from '@/account/AccountProvider'
import { useAccountCopy } from '@/lib/account-copy'
import { authURL } from '@/lib/account-return'
import { Button } from '@/components/ui/button'
export function AccountStatus({ children, target = '/account' }: { children: ReactNode; target?: string }) {
  const { state, store } = useAccount(); const { a } = useAccountCopy()
  if (state.status === 'signed-in') return children
  if (state.status === 'checking') return <p role="status">{a('checking')}</p>
  return <section className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8"><h2>{a(state.status === 'signed-out' ? 'signedOut' : 'unavailable')}</h2>
    <p className="text-muted-foreground">{a(state.status === 'signed-out' ? 'loginRequired' : 'recovery')}</p>
    <div className="flex flex-wrap items-start gap-3">{state.status === 'signed-out' ? <><Button asChild><a href={authURL('login', target)}>{a('login')}</a></Button><Button asChild variant="outline"><a href={authURL('register', target)}>{a('register')}</a></Button></> : <Button variant="outline" onClick={() => void store.revalidate()}>{a('retry')}</Button>}</div></section>
}

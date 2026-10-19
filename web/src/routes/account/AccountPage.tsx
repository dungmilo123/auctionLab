import { PageHeading, UnavailableState } from '@/components/site/Presentation'
import { useSiteCopy } from '@/lib/site-copy'
import { useAccountCopy } from '@/lib/account-copy'
import { useAccount } from '@/account/AccountProvider'
import type { Page } from '@/lib/route'
import AuthForm from './AuthForm'
import Profile from './Profile'
import Watchlist from './Watchlist'
import Bids from './Bids'
import { AccountStatus } from './AccountStatus'
export default function AccountPage({ page }: { page: Page }) {
  const { t } = useSiteCopy(); const { a } = useAccountCopy(); const { state } = useAccount()
  if (page === 'login' || page === 'register') return <AuthForm key={page} kind={page} />
  if (page === 'bids') return <><PageHeading title={t('bids')} intro={a('intro')}/><AccountStatus target="/account/bids">{state.status==='signed-in'&&<Bids key={state.profile.id}/>}</AccountStatus></>
  if (page === 'orders') return <><PageHeading title={t(page)} intro={a('intro')} /><UnavailableState title={t('unavailable')}>{t('ordersNote')}</UnavailableState></>
  return <><PageHeading title={page === 'watchlist' ? a('watchlist') : t('account')} intro={a('intro')} />
    <AccountStatus target={page === 'watchlist' ? '/account/watchlist' : '/account'}>
      {state.status === 'signed-in' && (page === 'watchlist' ? <Watchlist key={state.profile.id} /> : <Profile key={state.profile.id} profile={state.profile} />)}
    </AccountStatus></>
}

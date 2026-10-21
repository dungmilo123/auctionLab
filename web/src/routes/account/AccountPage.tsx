import { PageHeading } from '@/components/site/Presentation'
import { useSiteCopy } from '@/lib/site-copy'
import { useAccountCopy } from '@/lib/account-copy'
import { useAccount } from '@/account/AccountProvider'
import type { Page } from '@/lib/route'
import AuthForm from './AuthForm'
import Profile from './Profile'
import Watchlist from './Watchlist'
import Bids from './Bids'
import Wins from './Wins'
import Checkout from './Checkout'
import OrderHistory from './OrderHistory'
import { AccountStatus } from './AccountStatus'
export default function AccountPage({ page, orderId }: { page: Page; orderId?: string }) {
  const { t } = useSiteCopy(); const { a } = useAccountCopy(); const { state } = useAccount()
  if (page === 'login' || page === 'register') return <AuthForm key={page} kind={page} />
  if (page === 'wins' || page === 'checkout') return <><PageHeading title={t(page)} intro={a('intro')}/><AccountStatus target={page==='wins'?'/account/wins':`/account/checkout/${orderId}`}>
    {state.status==='signed-in'&&(page==='wins'?<Wins key={state.profile.id} owner={state.profile.id}/>:orderId&&<Checkout key={`${state.profile.id}:${orderId}`} owner={state.profile.id} orderID={orderId}/>)}</AccountStatus></>
  if (page === 'bids') return <><PageHeading title={t('bids')} intro={a('intro')}/><AccountStatus target="/account/bids">{state.status==='signed-in'&&<Bids key={state.profile.id}/>}</AccountStatus></>
  if (page === 'orders'||page==='orderDetail') return <><PageHeading title={t('orders')} intro={a('intro')}/><AccountStatus target={orderId?`/account/orders/${orderId}`:'/account/orders'}>{state.status==='signed-in'&&<OrderHistory key={`${state.profile.id}:${orderId??''}`} owner={state.profile.id} orderID={orderId}/>}</AccountStatus></>
  return <><PageHeading title={page === 'watchlist' ? a('watchlist') : t('account')} intro={a('intro')} />
    <AccountStatus target={page === 'watchlist' ? '/account/watchlist' : '/account'}>
      {state.status === 'signed-in' && (page === 'watchlist' ? <Watchlist key={state.profile.id} /> : <Profile key={state.profile.id} profile={state.profile} />)}
    </AccountStatus></>
}

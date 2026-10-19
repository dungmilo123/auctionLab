import { PageHeading, UnavailableState } from '@/components/site/Presentation'
import { useSiteCopy, type SiteKey } from '@/lib/site-copy'
import type { Page } from '@/lib/route'

export default function AccountPage({ page }: { page: Page }) {
  const { t } = useSiteCopy()
  const key: SiteKey = page === 'watchlist' || page === 'bids' || page === 'orders' ? page : 'account'
  const note: SiteKey = key === 'watchlist' ? 'watchlistNote' : key === 'bids' ? 'bidsNote' : key === 'orders' ? 'ordersNote' : 'accountIntro'
  return <><PageHeading title={t(key)} intro={t('accountIntro')} />
    <UnavailableState title={t('unavailable')}>{t(note)}</UnavailableState></>
}

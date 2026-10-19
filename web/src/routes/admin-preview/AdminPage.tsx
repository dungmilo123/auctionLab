import { PageHeading, UnavailableState } from '@/components/site/Presentation'
import { useSiteCopy, type SiteKey } from '@/lib/site-copy'
import type { Page } from '@/lib/route'

export default function AdminPage({ page }: { page: Page }) {
  const { t } = useSiteCopy()
  const key: SiteKey = page === 'adminAuctions' || page === 'adminLots' || page === 'adminOrders' ? page : 'admin'
  const note: SiteKey = key === 'adminAuctions' ? 'adminAuctionsNote' : key === 'adminLots' ? 'adminLotsNote' : key === 'adminOrders' ? 'adminOrdersNote' : 'adminIntro'
  return <><PageHeading title={t(key)} intro={t('adminIntro')} />
    <UnavailableState title={t('unavailable')}>{t(note)}</UnavailableState></>
}

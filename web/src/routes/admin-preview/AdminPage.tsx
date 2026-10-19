import { PageHeading, UnavailableState } from '@/components/site/Presentation'
import { useSiteCopy, type SiteKey } from '@/lib/site-copy'
import CatalogPage from '@/routes/admin/CatalogPage'
import type { Page } from '@/lib/route'

export default function AdminPage({ page }: { page: Page }) {
  const { t } = useSiteCopy()
  if (page === 'adminLots') return <CatalogPage kind="lots" />
  if (page === 'adminAuctions') return <CatalogPage kind="events" />
  const key: SiteKey = page === 'adminOrders' ? page : 'admin'
  const note: SiteKey = key === 'adminOrders' ? 'adminOrdersNote' : 'adminIntro'
  return <><PageHeading title={t(key)} intro={t('adminIntro')} />
    <UnavailableState title={t('unavailable')}>{t(note)}</UnavailableState>{page === 'admin' && <div className="catalog-actions"><a className="site-button site-button-outline" href="/admin/lots">{t('adminLots')}</a><a className="site-button site-button-outline" href="/admin/auctions">{t('adminAuctions')}</a></div>}</>
}

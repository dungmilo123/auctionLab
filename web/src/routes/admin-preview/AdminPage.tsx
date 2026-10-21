import { PageHeading } from '@/components/site/Presentation'
import { useSiteCopy } from '@/lib/site-copy'
import CatalogPage from '@/routes/admin/CatalogPage'
import AnalyticsPage from '@/routes/admin/AnalyticsPage'
import OrdersPage from '@/routes/admin/OrdersPage'
import type { Page } from '@/lib/route'

export default function AdminPage({ page }: { page: Page }) {
  const { t } = useSiteCopy()
  if (page === 'adminAnalytics') return <AnalyticsPage/>
  if (page === 'adminOrders') return <OrdersPage/>
  if (page === 'adminLots') return <CatalogPage kind="lots" />
  if (page === 'adminAuctions') return <CatalogPage kind="events" />
  return <><PageHeading title={t('admin')} intro={t('adminIntro')} />{page === 'admin' && <div className="catalog-actions"><a className="site-button site-button-outline" href="/admin/analytics">{t('analytics')}</a><a className="site-button site-button-outline" href="/admin/orders">{t('adminOrders')}</a><a className="site-button site-button-outline" href="/admin/lots">{t('adminLots')}</a><a className="site-button site-button-outline" href="/admin/auctions">{t('adminAuctions')}</a></div>}</>
}

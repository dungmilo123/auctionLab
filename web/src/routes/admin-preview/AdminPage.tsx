import { BarChart3, BookOpen, Gavel, Package } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
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
  return <><PageHeading title={t('admin')} intro={t('adminIntro')} />{page === 'admin' && <div className="admin-destinations"><a className="site-button site-button-outline" href="/admin/analytics"><Icon icon={BarChart3} size={24} />{t('analytics')}</a><a className="site-button site-button-outline" href="/admin/orders"><Icon icon={Package} size={24} />{t('adminOrders')}</a><a className="site-button site-button-outline" href="/admin/lots"><Icon icon={BookOpen} size={24} />{t('adminLots')}</a><a className="site-button site-button-outline" href="/admin/auctions"><Icon icon={Gavel} size={24} />{t('adminAuctions')}</a></div>}</>
}

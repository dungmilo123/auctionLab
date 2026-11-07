import AdminDashboard from '@/routes/admin/AdminDashboard'
import CatalogPage from '@/routes/admin/CatalogPage'
import AnalyticsPage from '@/routes/admin/AnalyticsPage'
import OrdersPage from '@/routes/admin/OrdersPage'
import LabsPage from '@/routes/admin/labs/LabsPage'
import type { Page } from '@/lib/route'

export default function AdminPage({ page }: { page: Page }) {
  if (page === 'adminLabs') return <LabsPage/>
  if (page === 'adminAnalytics') return <AnalyticsPage/>
  if (page === 'adminOrders') return <OrdersPage/>
  if (page === 'adminLots') return <CatalogPage kind="lots" />
  if (page === 'adminAuctions') return <CatalogPage kind="events" />
  return <AdminDashboard />
}

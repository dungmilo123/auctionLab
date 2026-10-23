import { useEffect, useState, lazy, Suspense } from 'react'
import { LanguageProvider, readStoredLang, writeStoredLang, type Lang } from '@/lib/i18n'
import { routeFor, type Page } from '@/lib/route'
import { siteText, type SiteKey } from '@/lib/site-copy'
import { AccountLayout, AdminLayout, StorefrontLayout } from '@/layouts/SiteLayout'
import StorefrontPage from '@/routes/storefront/StorefrontPage'
import { WatchlistProvider } from '@/account/WatchlistProvider'
import { AccountProvider } from '@/account/AccountProvider'
import PageBoundary from '@/components/site/PageBoundary'
const AccountPage = lazy(() => import('@/routes/account/AccountPage'))
const AdminPage = lazy(() => import('@/routes/admin-preview/AdminPage'))

const titles: Record<Page, SiteKey> = {
  home: 'homeTitle', auctions: 'auctions', auctionDetail: 'auctionDetail', lots: 'lots', lotDetail: 'lotDetail',
  account: 'account', login: 'login', register: 'register', watchlist: 'watchlist', bids: 'bids', wins: 'wins', checkout: 'checkout', orders: 'orders', orderDetail: 'orders',
  admin: 'admin', adminAuctions: 'adminAuctions', adminLots: 'adminLots', adminOrders: 'adminOrders', adminAnalytics: 'analytics', notFound: 'notFound',
}

export default function App() {
  const route = routeFor(window.location.pathname)
  const [lang, setLang] = useState<Lang>(() => readStoredLang())
  useEffect(() => {
    document.documentElement.lang = lang
    document.title = `${siteText(lang, titles[route.page])} · AuctionLab`
  }, [lang, route.page])
  function changeLang(next: Lang) { setLang(next); writeStoredLang(next) }
  const Layout = route.layout === 'account' ? AccountLayout : route.layout === 'admin' ? AdminLayout : StorefrontLayout
  return <LanguageProvider lang={lang} setLang={changeLang}>
    <AccountProvider><WatchlistProvider><Layout route={route}>
      <PageBoundary lang={lang}><Suspense fallback={<p role="status">{siteText(lang, 'loading')}</p>}>
      {route.layout === 'account' ? <AccountPage page={route.page} orderId={route.orderId} />
        : route.layout === 'admin' ? <AdminPage page={route.page} />
          : <StorefrontPage route={route} />}
      </Suspense></PageBoundary>
    </Layout></WatchlistProvider></AccountProvider>
  </LanguageProvider>
}

import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { PreviewNotice } from '@/components/site/Presentation'
import { useSiteCopy, type SiteKey } from '@/lib/site-copy'
import { useCatalogCopy } from '@/lib/catalog-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import type { Route } from '@/lib/route'

type Destination = readonly [href: string, label: SiteKey]
const primary: Destination[] = [['/auctions', 'auctions'], ['/lots', 'lots'], ['/account', 'account']]
const account: Destination[] = [['/account', 'overview'], ['/account/watchlist', 'watchlist'], ['/account/bids', 'bids'], ['/account/wins', 'wins'], ['/account/orders', 'orders']]
const admin: Destination[] = [['/admin', 'overview'], ['/admin/auctions', 'auctions'], ['/admin/lots', 'lots'], ['/admin/orders', 'orders']]

function Navigation({ items, path, label, className = 'site-nav' }: {
  items: Destination[]; path: string; label: SiteKey; className?: string
}) {
  const { t } = useSiteCopy()
  return <nav className={className} aria-label={t(label)}>
    {items.map(([href, key]) => <a key={href} href={href} aria-current={path === href ? 'page' : undefined}>{t(key)}</a>)}
  </nav>
}

function SiteLayout({ route, children }: { route: Route; children: ReactNode }) {
  const { lang, setLang, t } = useSiteCopy()
  const { c } = useCatalogCopy()
  const { d } = useDiscoveryCopy()
  return <div className="website">
    <a href="#main" className="skip-link">{t('skip')}</a>
    <header className="site-header">
      <div className="site-container header-inner">
        <a className="brand" href="/" aria-current={route.path === '/' ? 'page' : undefined}>
          <span className="brand-mark" aria-hidden="true">A</span>AuctionLab
        </a>
        <Navigation items={primary} path={route.path} label="primaryNav" />
        <div className="language-control" role="group" aria-label={t('language')}>
          {(['en', 'vi'] as const).map(code => <Button key={code} type="button" variant="ghost"
            aria-pressed={lang === code} aria-label={code === 'en' ? 'English' : 'Tiếng Việt'}
            lang={code} onClick={() => setLang(code)}>{code.toUpperCase()}</Button>)}
        </div>
      </div>
    </header>
    {route.page === 'adminLots' || route.page === 'adminAuctions' ? <div className="site-container preview-notice"><strong>{c('management')}</strong><span>{c('notice')}</span></div> : route.layout === 'storefront' ? <div className="site-container preview-notice"><strong>{d('published')}</strong><span>{d('notice')}</span></div> : <PreviewNotice />}
    <main id="main" tabIndex={-1} className="site-container site-main">
      {route.layout === 'storefront' ? children : <div className="context-layout">
        <Navigation items={route.layout === 'account' ? account : admin} path={route.path}
          label={route.layout === 'account' ? 'accountNav' : 'adminNav'} className="context-nav" />
        <div className="stack">{children}<a className="site-button site-button-link" href="/">{t('homeLink')}</a></div>
      </div>}
    </main>
    <footer className="site-footer">
      <div className="site-container footer-inner">
        <div className="stack-small"><p className="eyebrow">AuctionLab · {t('concept')}</p><p>{t('footerNote')}</p></div>
        <Navigation items={primary} path={route.path} label="footerNav" />
      </div>
    </footer>
  </div>
}

export function StorefrontLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }
export function AccountLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }
export function AdminLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }

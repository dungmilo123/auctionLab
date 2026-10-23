import { useRef, useState, type ReactNode } from 'react'
import { BarChart3, BookOpen, Gavel, Heart, Home, Menu, Package, Search, User, X } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import * as m from 'motion/react-m'
import { AnimatePresence, useIsPresent } from 'motion/react'
import { Icon } from '@/components/site/Icon'
import LanguageMenu from '@/components/site/LanguageMenu'
import { WebsiteMotion, useWebsiteReducedMotion } from '@/components/site/motion'
import { useSiteCopy, type SiteKey } from '@/lib/site-copy'
import type { Route } from '@/lib/route'

type Destination = readonly [href: string, label: SiteKey]
const primary: Destination[] = [['/auctions', 'auctions'], ['/lots', 'lots']]
const footer: Destination[] = [...primary, ['/account', 'account']]
const account: Destination[] = [['/account', 'overview'], ['/account/watchlist', 'watchlist'], ['/account/bids', 'bids'], ['/account/wins', 'wins'], ['/account/orders', 'orders']]
const admin: Destination[] = [['/admin', 'overview'], ['/admin/auctions', 'auctions'], ['/admin/lots', 'lots'], ['/admin/orders', 'orders'], ['/admin/analytics', 'analytics']]
const icons: Partial<Record<SiteKey, LucideIcon>> = { overview: Home, auctions: Gavel, lots: BookOpen, account: User, watchlist: Heart, bids: Gavel, wins: Package, orders: Package, analytics: BarChart3 }
function Navigation({ items, path, label, className = 'site-nav', icon = false }: { items: Destination[]; path: string; label: SiteKey; className?: string; icon?: boolean }) {
  const { t } = useSiteCopy()
  return <nav className={className} aria-label={t(label)}>{items.map(([href, key]) => <a key={href} href={href} aria-current={path === href ? 'page' : undefined}>{icon && icons[key] && <Icon icon={icons[key]!} />}{t(key)}</a>)}</nav>
}
function HeaderPanel({ children }: { children: ReactNode }) {
  const present = useIsPresent(), reduce = useWebsiteReducedMotion()
  return <m.div className="header-panel" inert={!present} initial={{ opacity: reduce ? 1 : 0, y: reduce ? 0 : -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: reduce ? 0 : -6 }}>{children}</m.div>
}
function Shell({ route, children }: { route: Route; children: ReactNode }) {
  const { lang, t } = useSiteCopy()
  const [panel, setPanel] = useState<'menu' | 'search' | null>(null)
  const menu = useRef<HTMLButtonElement>(null), search = useRef<HTMLButtonElement>(null)
  const reduce = useWebsiteReducedMotion()
  const close = () => { (panel === 'menu' ? menu : search).current?.focus(); setPanel(null) }
  return <div className="website" data-reduced-motion={reduce} onKeyDown={e => { if (e.key === 'Escape' && panel) close() }}>
    <a href="#main" className="skip-link">{t('skip')}</a>
    <header className="site-header"><div className="site-container header-inner">
      <a className="brand" href="/" aria-current={route.path === '/' ? 'page' : undefined}><span className="brand-mark" aria-hidden="true">A</span>AuctionLab</a>
      <div className="desktop-nav"><Navigation items={primary} path={route.path} label="primaryNav" /></div>
      <div className="header-tools">
        <button ref={search} type="button" className="site-button site-button-ghost header-search-trigger" aria-label={t('search')} aria-expanded={panel === 'search'} aria-controls="header-search" onClick={() => setPanel(panel === 'search' ? null : 'search')}><Icon icon={Search} /></button>
        <a className="site-button site-button-ghost desktop-tool" href="/account/watchlist" aria-label={t('watchlist')}><Icon icon={Heart} /></a>
        <a className="site-button site-button-ghost desktop-tool" href="/account" aria-label={t('account')}><Icon icon={User} /></a>
        <LanguageMenu />
        <button ref={menu} className="site-button site-button-ghost mobile-menu-trigger" type="button" aria-label={lang === 'vi' ? 'Điều hướng' : 'Navigation'} aria-expanded={panel === 'menu'} aria-controls="mobile-navigation" onClick={() => setPanel(panel === 'menu' ? null : 'menu')}><Icon icon={panel === 'menu' ? X : Menu} /></button>
      </div>
      <AnimatePresence initial={false}>
      {panel === 'search' && <HeaderPanel key="search"><form id="header-search" role="search" action="/lots" method="get" className="header-search">
        <label htmlFor="site-search">{t('search')}</label><div className="search-row"><input autoFocus id="site-search" className="site-input" type="search" name="q" defaultValue={new URLSearchParams(window.location.search).get('q') ?? ''} /><button className="site-button site-button-primary" type="submit"><Icon icon={Search} />{t('search')}</button></div>
      </form></HeaderPanel>}
      {panel === 'menu' && <HeaderPanel key="menu"><div id="mobile-navigation" className="mobile-navigation"><Navigation items={[...footer, ['/account/watchlist', 'watchlist']]} path={route.path} label="primaryNav" icon /></div></HeaderPanel>}
      </AnimatePresence>
    </div></header>
    <main id="main" tabIndex={-1} className="site-container site-main">{route.layout === 'storefront' ? children : <div className="context-layout"><Navigation items={route.layout === 'account' ? account : admin} path={route.page === 'orderDetail' || route.page === 'checkout' ? '/account/orders' : route.path} label={route.layout === 'account' ? 'accountNav' : 'adminNav'} className="context-nav" icon /><div className="stack">{children}<a className="site-button site-button-link" href="/"><Icon icon={Home} />{t('homeLink')}</a></div></div>}</main>
    <footer className="site-footer"><div className="site-container footer-inner"><div className="stack-small"><p className="footer-brand">{t('concept')}</p><p>{t('footerNote')}</p></div><Navigation items={footer} path={route.path} label="footerNav" /></div></footer>
  </div>
}
function SiteLayout(props: { route: Route; children: ReactNode }) { return <WebsiteMotion><Shell {...props} /></WebsiteMotion> }
export function StorefrontLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }
export function AccountLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }
export function AdminLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }

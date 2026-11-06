import { useRef, useState, type ReactNode } from 'react'
import { BarChart3, BookOpen, Gavel, Heart, Home, Menu, Package, Search, User, X } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import * as m from 'motion/react-m'
import { AnimatePresence, useIsPresent } from 'motion/react'
import { cn } from 'cn'
import { Icon } from '@/components/site/Icon'
import LanguageMenu from '@/components/site/LanguageMenu'
import { WebsiteMotion, useWebsiteReducedMotion } from '@/components/site/motion'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSiteCopy, type SiteKey } from '@/lib/site-copy'
import type { Route } from '@/lib/route'

type Destination = readonly [href: string, label: SiteKey]
const primary: Destination[] = [['/auctions', 'auctions'], ['/lots', 'lots']]
const footer: Destination[] = [...primary, ['/account', 'account']]
const account: Destination[] = [['/account', 'overview'], ['/account/watchlist', 'watchlist'], ['/account/bids', 'bids'], ['/account/wins', 'wins'], ['/account/orders', 'orders']]
const admin: Destination[] = [['/admin', 'overview'], ['/admin/auctions', 'auctions'], ['/admin/lots', 'lots'], ['/admin/orders', 'orders'], ['/admin/analytics', 'analytics']]
const icons: Partial<Record<SiteKey, LucideIcon>> = { overview: Home, auctions: Gavel, lots: BookOpen, account: User, watchlist: Heart, bids: Gavel, wins: Package, orders: Package, analytics: BarChart3 }

function Navigation({ items, path, label, className, icon = false, tone = 'ink' }: { items: Destination[]; path: string; label: SiteKey; className?: string; icon?: boolean; tone?: 'ink' | 'paper' }) {
  const { t } = useSiteCopy()
  return <nav className={cn('flex flex-wrap', className)} aria-label={t(label)}>{items.map(([href, key]) => <a key={href} href={href} aria-current={path === href ? 'page' : undefined} className={cn('inline-flex min-h-11 items-center font-mono text-[0.6875rem] font-medium tracking-[0.12em] uppercase no-underline hover:underline hover:underline-offset-[6px] aria-[current=page]:underline aria-[current=page]:decoration-2 aria-[current=page]:underline-offset-[6px]', tone === 'paper' ? 'text-paper' : 'text-ink')}>{icon && icons[key] && <Icon icon={icons[key]!} className="me-3" />}{t(key)}</a>)}</nav>
}
function HeaderPanel({ children }: { children: ReactNode }) {
  const present = useIsPresent(), reduce = useWebsiteReducedMotion()
  return <m.div className="min-w-0 basis-full" inert={!present} initial={{ opacity: reduce ? 1 : 0, y: reduce ? 0 : -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: reduce ? 0 : -6 }}>{children}</m.div>
}
function Shell({ route, children }: { route: Route; children: ReactNode }) {
  const { lang, t } = useSiteCopy()
  const [panel, setPanel] = useState<'menu' | 'search' | null>(null)
  const menu = useRef<HTMLButtonElement>(null), search = useRef<HTMLButtonElement>(null)
  const reduce = useWebsiteReducedMotion()
  const close = () => { (panel === 'menu' ? menu : search).current?.focus(); setPanel(null) }
  return <div className="website flex min-h-dvh flex-col [overflow-wrap:anywhere]" data-reduced-motion={reduce} onKeyDown={e => { if (e.key === 'Escape' && panel) close() }}>
    <a href="#main" className="fixed top-[-100%] left-4 z-10 min-h-11 rounded-[2px] border border-ink bg-paper px-3 py-3 focus:top-2">{t('skip')}</a>
    <header className="sticky top-0 z-40 border-b border-hairline bg-[#EAEAE7DB] backdrop-blur-md backdrop-saturate-150">
      <div className="mx-auto flex w-full max-w-[1440px] flex-wrap items-center justify-between gap-x-6 gap-y-2 px-4 py-2.5 sm:px-6 lg:px-8">
        <a className="inline-flex min-h-11 items-center gap-2.5 font-display text-base font-semibold tracking-[0.01em] uppercase text-ink no-underline hover:no-underline" href="/" aria-current={route.path === '/' ? 'page' : undefined}><span className="size-3 shrink-0 rounded-[1px] bg-ink" aria-hidden="true" />AuctionLab</a>
        <div className="hidden md:block"><Navigation items={primary} path={route.path} label="primaryNav" className="gap-x-7" /></div>
        <div className="flex items-center gap-0.5">
          <Button ref={search} type="button" variant="ghost" size="icon" aria-label={t('search')} aria-expanded={panel === 'search'} aria-controls="header-search" onClick={() => setPanel(panel === 'search' ? null : 'search')}><Icon icon={Search} /></Button>
          <Button asChild variant="ghost" size="icon" className="hidden md:inline-flex"><a href="/account/watchlist" aria-label={t('watchlist')}><Icon icon={Heart} /></a></Button>
          <Button asChild variant="ghost" size="icon" className="hidden md:inline-flex"><a href="/account" aria-label={t('account')}><Icon icon={User} /></a></Button>
          <LanguageMenu />
          <Button ref={menu} variant="ghost" size="icon" className="md:hidden" type="button" aria-label={lang === 'vi' ? 'Điều hướng' : 'Navigation'} aria-expanded={panel === 'menu'} aria-controls="mobile-navigation" onClick={() => setPanel(panel === 'menu' ? null : 'menu')}><Icon icon={panel === 'menu' ? X : Menu} /></Button>
        </div>
        <AnimatePresence initial={false}>
        {panel === 'search' && <HeaderPanel key="search"><form id="header-search" role="search" action="/lots" method="get" className="border-t border-hairline py-4">
          <Label htmlFor="site-search">{t('search')}</Label><div className="flex gap-3"><Input autoFocus id="site-search" className="flex-1" type="search" name="q" defaultValue={new URLSearchParams(window.location.search).get('q') ?? ''} /><Button type="submit"><Icon icon={Search} />{t('search')}</Button></div>
        </form></HeaderPanel>}
        {panel === 'menu' && <HeaderPanel key="menu"><div id="mobile-navigation" className="py-4"><Navigation items={[...footer, ['/account/watchlist', 'watchlist']]} path={route.path} label="primaryNav" className="grid gap-y-1" icon /></div></HeaderPanel>}
        </AnimatePresence>
      </div>
    </header>
    <main id="main" tabIndex={-1} className="mx-auto w-full max-w-[1440px] flex-1 px-4 pt-10 pb-24 focus:outline-none focus:shadow-none sm:px-6 lg:px-8">{route.layout === 'storefront' ? children : <div className="grid gap-8 lg:grid-cols-[220px_minmax(0,1fr)] lg:gap-12"><Navigation items={route.layout === 'account' ? account : admin} path={route.page === 'orderDetail' || route.page === 'checkout' ? '/account/orders' : route.path} label={route.layout === 'account' ? 'accountNav' : 'adminNav'} className="content-start gap-x-4 gap-y-1 border-b border-hairline pb-6 lg:flex-col lg:border-r lg:border-b-0 lg:pr-6" icon /><div className="flex min-w-0 flex-col gap-6">{children}<a className="inline-flex min-h-11 items-center font-mono text-[0.6875rem] tracking-[0.12em] uppercase text-ink underline underline-offset-4" href="/"><Icon icon={Home} className="me-2" />{t('homeLink')}</a></div></div>}</main>
    <footer className="border-t border-ink bg-ink py-10 text-[0.8125rem] text-paper/70 sm:py-12"><div className="mx-auto flex w-full max-w-[1440px] flex-wrap justify-between gap-8 px-4 sm:px-6 lg:px-8"><div className="flex flex-col gap-2.5"><p className="font-display text-2xl font-medium tracking-[-0.04em] uppercase leading-none text-paper">{t('concept')}</p><p className="max-w-[34rem]">{t('footerNote')}</p></div><Navigation items={footer} path={route.path} label="footerNav" className="gap-x-6" tone="paper" /></div></footer>
  </div>
}
function SiteLayout(props: { route: Route; children: ReactNode }) { return <WebsiteMotion><Shell {...props} /></WebsiteMotion> }
export function StorefrontLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }
export function AccountLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }
export function AdminLayout(props: { route: Route; children: ReactNode }) { return <SiteLayout {...props} /> }

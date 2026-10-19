export type Page = 'home' | 'auctions' | 'auctionDetail' | 'lots' | 'lotDetail'
  | 'account' | 'watchlist' | 'bids' | 'orders'
  | 'admin' | 'adminAuctions' | 'adminLots' | 'adminOrders' | 'notFound'
export type Layout = 'storefront' | 'account' | 'admin'
export type Route = { page: Page; layout: Layout; path: string; slug?: string }

export const SITE_ROUTES = [
  ['/', 'home', 'storefront'],
  ['/auctions', 'auctions', 'storefront'],
  ['/lots', 'lots', 'storefront'],
  ['/account', 'account', 'account'],
  ['/account/watchlist', 'watchlist', 'account'],
  ['/account/bids', 'bids', 'account'],
  ['/account/orders', 'orders', 'account'],
  ['/admin', 'admin', 'admin'],
  ['/admin/auctions', 'adminAuctions', 'admin'],
  ['/admin/lots', 'adminLots', 'admin'],
  ['/admin/orders', 'adminOrders', 'admin'],
] as const satisfies ReadonlyArray<readonly [string, Page, Layout]>

/** Native document links; exactly one trailing slash is accepted. */
export function routeFor(input: string): Route {
  const pathname = input.split(/[?#]/, 1)[0] ?? ''
  const path = pathname !== '/' && pathname !== '//' && pathname.endsWith('/') ? pathname.slice(0, -1) : pathname
  const exact = SITE_ROUTES.find(([href]) => href === path)
  if (exact) return { path, page: exact[1], layout: exact[2] }
  const detail = /^\/(auctions|lots)\/([^/]+)$/.exec(path)
  if (detail) return {
    path, layout: 'storefront',
    page: detail[1] === 'lots' ? 'lotDetail' : 'auctionDetail', slug: detail[2],
  }
  return { path, page: 'notFound', layout: 'storefront' }
}

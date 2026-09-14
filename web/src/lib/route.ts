export type Route = 'player' | 'admin' | 'screen'

/**
 * Pure route selector — deliberately no router dependency.
 *
 * Anything after the first `?` or `#` is ignored before matching. `/admin` and
 * `/admin/**` map to admin, `/screen` and `/screen/**` to screen, and everything
 * else — including near-misses such as `/administrator` — to player.
 *
 * The app shell switches on this value; a router can replace it later without
 * touching the views.
 */
export function routeFor(pathname: string): Route {
  const path = pathname.split(/[?#]/, 1)[0] ?? '/'
  if (path === '/admin' || path.startsWith('/admin/')) return 'admin'
  if (path === '/screen' || path.startsWith('/screen/')) return 'screen'
  return 'player'
}

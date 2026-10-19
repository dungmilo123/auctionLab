import { routeFor } from './route'
/** Validate a URL value after URLSearchParams has decoded the outer query. */
export function safeReturn(value: string | null | undefined): string {
  if (!value || !value.startsWith('/') || value.startsWith('//') || [...value].some(c => c === '\\' || c.charCodeAt(0) <= 32 || c.charCodeAt(0) === 127)) return '/account'
  let decoded: string
  try { decoded = decodeURIComponent(value) } catch { return '/account' }
  if ([...decoded].some(c => c === '\\' || c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127)) return '/account'
  const path = (value.split(/[?#]/, 1)[0] ?? '')
  if (path.includes('%') || path.split('/').some(s => s === '.' || s === '..')) return '/account'
  const route = routeFor(path)
  if (route.page === 'notFound' || route.layout === 'admin' || route.page === 'login' || route.page === 'register') return '/account'
  return value.split('#', 1)[0] ?? '/account'
}
export function authURL(kind: 'login' | 'register', target: string): string {
  return `/account/${kind}?return_to=${encodeURIComponent(safeReturn(target))}`
}

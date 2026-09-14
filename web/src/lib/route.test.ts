import { describe, expect, it } from 'vitest'
import { routeFor, type Route } from './route'

// The full case table from the phase plan: three top-level routes, their deep
// links, the near-miss prefixes that must NOT match, and query/fragment
// stripping.
const cases: Array<[pathname: string, expected: Route]> = [
  ['/', 'player'],
  ['/admin', 'admin'],
  ['/admin/', 'admin'],
  ['/admin/deep/link', 'admin'],
  ['/screen', 'screen'],
  ['/screen/', 'screen'],
  ['/screen/any/deep', 'screen'],
  ['/administrator', 'player'],
  ['/screensaver', 'player'],
  ['/foo', 'player'],
]

describe('routeFor', () => {
  it.each(cases)('maps %s to %s', (pathname, expected) => {
    expect(routeFor(pathname)).toBe(expected)
  })

  it('ignores a query string and a fragment when selecting', () => {
    expect(routeFor('/admin?k=secret')).toBe('admin')
    expect(routeFor('/screen#podium')).toBe('screen')
  })
})

import { expect, it } from 'vitest'
import { safeReturn, authURL } from './account-return'
it('preserves recognized native destinations and ordinary discovery queries', () => {
  for (const value of ['/', '/account/watchlist', '/account/bids', '/lots/camera/', '/auctions/event', '/lots?q=m%C3%A1y+%E1%BA%A3nh&sort=price-asc']) expect(safeReturn(value)).toBe(value)
  expect(safeReturn('/lots/camera#detail')).toBe('/lots/camera')
  expect(authURL('login', '/lots/camera')).toBe('/account/login?return_to=%2Flots%2Fcamera')
})
it('rejects external, ambiguous, encoded and looping destinations', () => {
  for (const value of [null, '', 'https://evil.test', '//evil.test', '/\\evil', '/lots/%2fcamera', '/%2f%2fevil', '/lots/%252fcamera', '/lots/camera?x=%0a', '/lots/camera?x=%5c', '/lots/camera?x=%zz', '/lots/../admin', '/admin', '/api/account', '/ws', '/assets/a.js', '/account/login', '/account/register/', '/account/login/extra', '/lots/camera//', '/lots/camera\n', '/Lots']) expect(safeReturn(value)).toBe('/account')
})

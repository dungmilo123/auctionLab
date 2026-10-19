import { describe, expect, it } from 'vitest'
import { routeFor, SITE_ROUTES } from './route'

describe('website route resolution', () => {
  it.each(SITE_ROUTES)('resolves %s explicitly', (path, page, layout) => {
    expect(routeFor(path)).toEqual({ path, page, layout })
    if (path !== '/') expect(routeFor(`${path}/`)).toEqual({ path, page, layout })
    expect(routeFor(`${path}?view=preview#details`)).toEqual({ path, page, layout })
  })
  it.each([['/auctions/anything', 'auctionDetail'], ['/lots/sample-camera', 'lotDetail']] as const)(
    'accepts one generic slug at %s', (path, page) => {
      expect(routeFor(`${path}/?view=preview#details`)).toEqual({ path, page, layout: 'storefront', slug: path.split('/')[2] })
    },
  )
  it.each([
    '', '//', '/lots//', '/lots//camera', '/lots/sample-camera/extra', '/lots/camera//',
    '/auctions/x/y', '/auctions//', '/Lots', '/Account', '/ADMIN',
    '/administrator', '/accounts', '/lotsmore', '/screensaver',
    '/account/unknown', '/account/orders/1', '/admin/unknown', '/admin/lots/1',
    '/screen', '/screen/', '/screen/anything/deep', '/game', '/legacy', '/login', '/checkout',
    '/foo',
  ])('returns a storefront 404 for %s', (path) => {
    expect(routeFor(path)).toMatchObject({ page: 'notFound', layout: 'storefront' })
  })
})

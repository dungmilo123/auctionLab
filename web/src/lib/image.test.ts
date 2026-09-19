/**
 * D-01's resolution rules, pinned as a table: `itemImageSrc` is the ONE place a
 * stored image value becomes a URL, so these cases are the contract every item
 * view renders through (the player bodies, the projector cards and the admin
 * thumbnail all call it).
 */
import { describe, expect, it } from 'vitest'
import { itemImageSrc } from './image'

describe('itemImageSrc (D-01)', () => {
  const preserved: Array<[label: string, raw: string, expected: string]> = [
    ['an https URL', 'https://cdn.example.com/lots/vase.png', 'https://cdn.example.com/lots/vase.png'],
    ['an http URL', 'http://192.168.0.103:8080/pics/vase.png', 'http://192.168.0.103:8080/pics/vase.png'],
    ['a data URL', 'data:image/png;base64,iVBORw0KGgo=', 'data:image/png;base64,iVBORw0KGgo='],
    ['a root-relative path', '/assets/vase.png', '/assets/vase.png'],
    ['an already-local path', '/images/vase.png', '/images/vase.png'],
  ]

  it.each(preserved)('preserves %s', (_label, raw, expected) => {
    expect(itemImageSrc(raw)).toBe(expected)
  })

  const local: Array<[label: string, raw: string, expected: string]> = [
    ['a bare file name', 'vase.png', '/images/vase.png'],
    ['a name with a ./ prefix', './vase.png', '/images/vase.png'],
    ['a name already under images/', 'images/vase.png', '/images/vase.png'],
    ['a doubly-prefixed name', './images/vase.png', '/images/vase.png'],
    ['a name with spaces inside', 'bức tranh.png', '/images/bức tranh.png'],
  ]

  it.each(local)('resolves %s under /images/', (_label, raw, expected) => {
    expect(itemImageSrc(raw)).toBe(expected)
  })

  it('returns an empty string for an empty or blank value, so nothing renders', () => {
    expect(itemImageSrc('')).toBe('')
    expect(itemImageSrc('   ')).toBe('')
  })

  it('trims surrounding whitespace before deciding', () => {
    expect(itemImageSrc('  vase.png  ')).toBe('/images/vase.png')
    expect(itemImageSrc('  https://cdn.example.com/vase.png  ')).toBe(
      'https://cdn.example.com/vase.png',
    )
  })

  it('never invents a scheme, so an unsupported one becomes a path, not a URL', () => {
    // The whitelist is the point: only http/https/data/leading-slash values are
    // passed through, and everything else is treated as a file name under the
    // server's own /images/ route.
    expect(itemImageSrc('javascript:alert(1)')).toBe('/images/javascript:alert(1)')
    expect(itemImageSrc('ftp://host/vase.png')).toBe('/images/ftp://host/vase.png')
  })
})

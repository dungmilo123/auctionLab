import { describe, expect, it } from 'vitest'
import { DISCOVERY_COPY } from './discovery-copy'
import { SITE_COPY, siteText } from './site-copy'

describe('website copy', () => {
  it('contains nonempty English/Vietnamese copy and matching interpolation tokens', () => {
    for (const [en, vi] of Object.values(DISCOVERY_COPY)) {expect(en.trim()).not.toBe('');expect(vi.trim()).not.toBe('')}
    for (const copy of Object.values(SITE_COPY)) {
      expect(copy.en.trim()).not.toBe('')
      expect(copy.vi.trim()).not.toBe('')
      expect(copy.en.match(/\{\w+\}/g) ?? []).toEqual(copy.vi.match(/\{\w+\}/g) ?? [])
    }
  })
  it('interpolates plain text in both languages, retaining missing tokens', () => {
    expect(siteText('en', 'imageAlt', { object: 'camera' })).toBe('Drawing of camera')
    expect(siteText('vi', 'imageAlt', { object: 'máy ảnh' })).toBe('Hình minh họa máy ảnh')
    expect(siteText('en', 'imageAlt')).toBe('Drawing of {object}')
    expect(siteText('en', 'imageAlt', { object: '<img>' })).toBe('Drawing of <img>')
  })
})

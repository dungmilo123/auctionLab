import { expect, it } from 'vitest'
import { ACCOUNT_COPY, accountText } from './account-copy'
it('keeps EN/VI account messages complete and token-compatible', () => {
  for (const [en, vi] of Object.values(ACCOUNT_COPY)) { expect(en.trim()).not.toBe(''); expect(vi.trim()).not.toBe(''); expect(en.match(/\{\w+\}/g) ?? []).toEqual(vi.match(/\{\w+\}/g) ?? []) }
  expect(accountText('en', 'count', { count: '2' })).toBe('2 saved lots')
  expect(accountText('vi', 'count', { count: '2' })).toBe('2 hiện vật đã lưu')
})

import { describe, expect, it } from 'vitest'
import { CATALOG_COPY } from './catalog-copy'
import { catalogSelection } from './catalog-selection'
import { catalogMoneyInput, formatCatalogMoney, MAX_CATALOG_MONEY, parseCatalogMoney } from './catalog-money'
import { localScheduleInput, parseLocalSchedule, parseInterval } from './catalog-schedule'
describe('catalog preparation helpers', () => {
  it('round trips exact cents independently of legacy whole-unit money', () => {
    for (const value of [0, 1, 12550, 500, MAX_CATALOG_MONEY]) expect(parseCatalogMoney(catalogMoneyInput(value))).toBe(value)
    expect(formatCatalogMoney(12550)).toBe('$125.50 USD')
    expect(parseCatalogMoney('')).toBe(null)
    for (const value of ['-1', '1.001', '1e3', 'NaN', ' 1', '1.', '10000000000.01']) expect(() => parseCatalogMoney(value)).toThrow()
  })
  it('preserves UTC instants through local controls, including milliseconds', () => {
    const value = Date.parse('2030-01-01T12:34:56.123Z')
    expect(parseLocalSchedule(localScheduleInput(value))).toBe(value)
    expect(localScheduleInput(null)).toBe('')
    expect(parseLocalSchedule('')).toBe(null)
    expect(parseInterval('')).toBe(null)
    for (const input of ['0', '-1', '1.1', '9007199254740993']) expect(() => parseInterval(input)).toThrow()
    for (const input of ['invalid', '2030-02-31T00:00', '1960-01-01T00:00']) expect(() => parseLocalSchedule(input)).toThrow()
  })
  it('selects only explicit existing-route editor queries', () => {
    expect(catalogSelection('')).toEqual({ kind: 'list' })
    expect(catalogSelection('?new=1')).toEqual({ kind: 'new' })
    expect(catalogSelection('?edit=demo-lot-01')).toEqual({ kind: 'edit', id: 'demo-lot-01' })
    for (const query of ['?edit=', '?edit=a&new=1', '?new=2', '?edit=a&edit=b', '?edit=%3Cscript%3E']) expect(catalogSelection(query)).toEqual({ kind: 'invalid' })
  })
  it('provides nonempty EN/VI labels and feedback with matching tokens', () => {
    for (const value of Object.values(CATALOG_COPY)) {
      expect(value.en.trim()).not.toBe(''); expect(value.vi.trim()).not.toBe('')
      expect(value.en.match(/\{\w+\}/g) ?? []).toEqual(value.vi.match(/\{\w+\}/g) ?? [])
    }
  })
})

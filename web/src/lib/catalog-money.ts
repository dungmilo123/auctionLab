export const MAX_CATALOG_MONEY = 1_000_000_000_000
/** Decimal text to cents; never rounds a binary floating input. Empty means absent. */
export function parseCatalogMoney(value: string): number | null {
  if (value === '') return null
  if (!/^\d+(?:\.\d{1,2})?$/.test(value)) throw new Error('money')
  const [whole, fraction = ''] = value.split('.')
  const minor = BigInt(whole!) * 100n + BigInt(fraction.padEnd(2, '0'))
  if (minor > BigInt(MAX_CATALOG_MONEY)) throw new Error('money')
  return Number(minor)
}
export function catalogMoneyInput(minor: number | null): string {
  if (minor === null) return ''
  if (!Number.isSafeInteger(minor) || minor < 0 || minor > MAX_CATALOG_MONEY) throw new Error('money')
  return `${Math.floor(minor / 100)}.${String(minor % 100).padStart(2, '0')}`
}
export function formatCatalogMoney(minor: number | null): string { return minor === null ? '—' : `$${catalogMoneyInput(minor)} USD` }

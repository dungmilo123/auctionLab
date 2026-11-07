import type { Lang } from './i18n'

export const MAX_CATALOG_MONEY = 1_000_000_000_000
/**
 * The demonstration catalogue is priced in US dollars — the wire and the schema
 * fix the currency — and the Vietnamese interface renders the same amounts in
 * đồng at this one fixed rate. One dollar is 25,000 ₫, so one cent is exactly
 * 250 ₫: the conversion needs no rounding and no stored value ever changes with
 * the language. It is a display (and bid-input) transform only.
 */
export const VND_PER_USD = 25_000
const DONG_PER_CENT = VND_PER_USD / 100

function outOfRange(minor: number): boolean {
  return !Number.isSafeInteger(minor) || minor < 0 || minor > MAX_CATALOG_MONEY
}

/** Decimal text to cents; never rounds a binary floating input. Empty means absent. */
export function parseCatalogMoney(value: string): number | null {
  if (value === '') return null
  if (!/^\d+(?:\.\d{1,2})?$/.test(value)) throw new Error('money')
  const [whole, fraction = ''] = value.split('.')
  const minor = BigInt(whole!) * 100n + BigInt(fraction.padEnd(2, '0'))
  if (minor > BigInt(MAX_CATALOG_MONEY)) throw new Error('money')
  return Number(minor)
}

/**
 * Whole đồng to cents, for the Vietnamese bid form. At 250 ₫ to the cent only
 * amounts divisible by 250 are representable, so anything else is refused
 * rather than silently rounded; dots are Vietnamese grouping and are dropped.
 */
export function parseDongMoney(value: string): number | null {
  const digits = value.replace(/[.\s]/g, '')
  if (digits === '') return null
  if (!/^\d+$/.test(digits)) throw new Error('money')
  const dong = Number(digits)
  if (!Number.isSafeInteger(dong) || dong % DONG_PER_CENT !== 0) throw new Error('money')
  const minor = dong / DONG_PER_CENT
  if (outOfRange(minor)) throw new Error('money')
  return minor
}

export function catalogMoneyInput(minor: number | null): string {
  if (minor === null) return ''
  if (outOfRange(minor)) throw new Error('money')
  return `${Math.floor(minor / 100)}.${String(minor % 100).padStart(2, '0')}`
}

/** Cents to `4.350.000 ₫`: Vietnamese dot grouping, and no decimals (đồng has none). */
export function formatDong(minor: number): string {
  return `${String(minor * DONG_PER_CENT).replace(/\B(?=(\d{3})+(?!\d))/g, '.')} ₫`
}

/**
 * `lang` is optional and defaults to the catalogue's own currency, so the admin
 * console — whose money fields are USD decimals by design — is unaffected. The
 * customer-facing surfaces pass the interface language.
 */
export function formatCatalogMoney(minor: number | null, lang: Lang = 'en'): string {
  if (minor === null) return '—'
  if (outOfRange(minor)) throw new Error('money')
  return lang === 'vi' ? formatDong(minor) : `$${catalogMoneyInput(minor)} USD`
}

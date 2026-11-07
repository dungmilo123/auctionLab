import { MAX_CATALOG_MONEY, formatDong } from "./catalog-money";
import type { Lang } from './i18n'
export const MAX_ORDER_TOTAL = 2 * MAX_CATALOG_MONEY;
export function formatOrderMoney(minor: number, lang: Lang = 'en'): string {
  if (!Number.isSafeInteger(minor) || minor < 0 || minor > MAX_ORDER_TOTAL)
    throw new Error("money");
  return lang === "vi"
    ? formatDong(minor)
    : `$${Math.floor(minor / 100)}.${String(minor % 100).padStart(2, "0")} USD`;
}

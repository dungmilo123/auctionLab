import { MAX_CATALOG_MONEY } from "./catalog-money";
export const MAX_ORDER_TOTAL = 2 * MAX_CATALOG_MONEY;
export function formatOrderMoney(minor: number): string {
  if (!Number.isSafeInteger(minor) || minor < 0 || minor > MAX_ORDER_TOTAL)
    throw new Error("money");
  return `$${Math.floor(minor / 100)}.${String(minor % 100).padStart(2, "0")} USD`;
}

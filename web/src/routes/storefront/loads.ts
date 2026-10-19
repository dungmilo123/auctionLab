import { publicCatalog, type PublicResponse } from '@/api/public-catalog'
import type { Category } from '@/api/catalog'
export async function withCategories<T>(
  primary: Promise<PublicResponse<T>>,
  signal: AbortSignal,
) {
  const [record, categories] = await Promise.all([
    primary,
    publicCatalog.categories(signal),
  ])
  return {
    ...record,
    data: { record: record.data, categories: categories.data },
  }
}
export const loadHome = (signal: AbortSignal) =>
  withCategories(publicCatalog.events(signal), signal)
export const loadEvents = (signal: AbortSignal) => publicCatalog.events(signal)
export const loadLots = (signal: AbortSignal) =>
  withCategories(publicCatalog.lots(signal), signal)
export type Categorized<T> = { record: T; categories: Category[] }

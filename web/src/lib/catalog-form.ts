import { CatalogError } from '@/api/catalog'
export function moveItem<T>(values: T[], index: number, delta: number): T[] { const out = [...values]; const target = index + delta; if (index < 0 || target < 0 || index >= out.length || target >= out.length) return values; [out[index], out[target]] = [out[target]!, out[index]!]; return out }
export const asCatalogError = (error: unknown) => error instanceof CatalogError ? error : new CatalogError('storage_error')
export function requiresReload(error: CatalogError) { return ['commit_unknown', 'stale_version', 'transaction_conflict', 'published'].includes(error.code) }

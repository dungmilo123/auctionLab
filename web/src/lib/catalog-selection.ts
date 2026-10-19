export type CatalogSelection = { kind: 'list' } | { kind: 'new' } | { kind: 'edit'; id: string } | { kind: 'invalid' }
export function catalogSelection(search: string): CatalogSelection {
  const params = new URLSearchParams(search)
  if (params.has('edit') && params.has('new') || params.getAll('edit').length > 1 || params.getAll('new').length > 1) return { kind: 'invalid' }
  if (params.has('new')) return params.get('new') === '1' ? { kind: 'new' } : { kind: 'invalid' }
  if (params.has('edit')) { const id = params.get('edit'); return id && /^[a-zA-Z0-9_-]{1,128}$/.test(id) ? { kind: 'edit', id } : { kind: 'invalid' } }
  return { kind: 'list' }
}

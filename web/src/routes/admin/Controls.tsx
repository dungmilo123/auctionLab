import type { ReactNode } from 'react'
import { Field } from '@/components/site/Presentation'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { useCatalogCopy } from '@/lib/catalog-copy'
import { CatalogError } from '@/api/catalog'

export function Control({ name, label, help = '', error, value, onChange, type = 'text', children, readOnly = false }: {
  name: string; label: string; help?: string; error?: string; value: string; onChange: (value: string) => void; type?: 'text' | 'textarea' | 'select' | 'datetime-local'; children?: ReactNode; readOnly?: boolean
}) {
  const id = `catalog-${name.replaceAll('.', '-')}`
  if (type === 'text' || type === 'datetime-local') return <Field id={id} label={label} help={help} error={error} value={value} type={type} step={type === 'datetime-local' ? '0.001' : undefined} readOnly={readOnly} onChange={e => onChange(e.target.value)} />
  const props = { id, value, className: 'site-input', 'aria-invalid': error ? true as const : undefined, 'aria-describedby': `${id}-help${error ? ` ${id}-error` : ''}` }
  return <div><Label htmlFor={id}>{label}</Label>{type === 'textarea' ? <textarea {...props} rows={4} readOnly={readOnly} onChange={e => onChange(e.target.value)} /> : <select {...props} disabled={readOnly} onChange={e => onChange(e.target.value)}>{children}</select>}
    <p id={`${id}-help`} className="field-help">{help}</p>{error && <p id={`${id}-error`} className="field-error">{error}</p>}</div>
}
export function MoveControls({ index, length, label, move, remove }: { index: number; length: number; label: string; move: (delta: number) => void; remove: () => void }) {
  const { c } = useCatalogCopy()
  return <div className="catalog-actions"><Button type="button" variant="outline" disabled={index === 0} aria-label={`${c('up')}: ${label}`} onClick={() => move(-1)}>{c('up')}</Button><Button type="button" variant="outline" disabled={index === length - 1} aria-label={`${c('down')}: ${label}`} onClick={() => move(1)}>{c('down')}</Button><Button type="button" variant="ghost" aria-label={`${c('remove')}: ${label}`} onClick={remove}>{c('remove')}</Button></div>
}
export function ErrorPanel({ error, reload }: { error: CatalogError; reload?: () => void }) {
  const { c, errorText, fieldText } = useCatalogCopy()
  const gate = new URL('/admin', window.location.href)
  if (import.meta.env.DEV) gate.port = '8080'
  return <div className="catalog-error stack-small" role="alert"><p>{errorText(error.code)}</p>
    {error.code === 'access_required' && <><p>{c('accessHelp')}</p><a href={gate.href} target="_blank" rel="noreferrer">{c('gate')}</a></>}
    {Object.entries(error.fields).length > 0 && <ul>{Object.entries(error.fields).map(([path, code]) => { const member = /^lots\.([^.]+)\.(.+)$/.exec(path); return <li key={path}>{member ? <a href={`/admin/lots?edit=${encodeURIComponent(member[1]!)}`}>{c('lots')} {member[1]} · {fieldText(member[2]!)}</a> : <a href={`#catalog-${path.replaceAll('.', '-')}`}>{fieldText(path)}</a>}: {errorText(code)}</li> })}</ul>}
    {reload && <><p>{c('recoveryHelp')}</p><Button type="button" variant="outline" onClick={reload}>{c('reload')}</Button></>}
  </div>
}

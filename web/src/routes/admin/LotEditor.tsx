import { BookOpen, CheckCircle2, Save } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { LotImage } from '@/components/site/Presentation'
import { catalogAPI, CatalogError, lotContent, type Category, type Lot, type LotContent } from '@/api/catalog'
import { useCatalogCopy } from '@/lib/catalog-copy'
import { catalogMoneyInput, parseCatalogMoney } from '@/lib/catalog-money'
import { asCatalogError, moveItem, requiresReload } from '@/lib/catalog-form'
import { Control, ErrorPanel, MoveControls } from './Controls'

export default function LotEditor({ initial, categories, reload }: { initial: Lot; categories: Category[]; reload: () => void }) {
  const { c, lang, errorText } = useCatalogCopy()
  const [record, setRecord] = useState(initial)
  const [content, setContent] = useState(() => lotContent(initial))
  const [prices, setPrices] = useState(() => ({ starting_price_minor: catalogMoneyInput(initial.starting_price_minor), bid_increment_minor: catalogMoneyInput(initial.bid_increment_minor), shipping_fee_minor: catalogMoneyInput(initial.fulfillment.shipping_fee_minor) }))
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<CatalogError>()
  const [saved, setSaved] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [blocked, setBlocked] = useState(false)
  const immutable = record.state === 'published'
  function change<K extends keyof LotContent>(key: K, value: LotContent[K]) { setContent(old => ({ ...old, [key]: value })); setDirty(true); setSaved(false) }
  const fieldError = (name: string) => error?.fields[name] ? errorText(error.fields[name]) : undefined
  async function submit(event: FormEvent) {
    event.preventDefault(); setError(undefined); setSaved(false)
    const fields: Record<string, string> = {}; const amounts: Record<string, number | null> = {}
    for (const [key, value] of Object.entries(prices)) { try { amounts[key] = parseCatalogMoney(value) } catch { fields[key === 'shipping_fee_minor' ? `fulfillment.${key}` : key] = 'money' } }
    if (Object.keys(fields).length) { setError(new CatalogError('validation', 422, fields)); return }
    setPending(true)
    try {
      const next = await catalogAPI.saveLot(record, { ...content, starting_price_minor: amounts.starting_price_minor ?? null, bid_increment_minor: amounts.bid_increment_minor ?? null, fulfillment: { ...content.fulfillment, shipping_fee_minor: amounts.shipping_fee_minor ?? null } })
      setRecord(next); setContent(lotContent(next)); setDirty(false); setSaved(true)
    } catch (err) { const failure = asCatalogError(err); setError(failure); if (requiresReload(failure)) setBlocked(true) }
    finally { setPending(false) }
  }
  const simple = (name: 'title' | 'slug' | 'description' | 'brand' | 'era' | 'story' | 'condition' | 'admin_notes', textarea = false) => <Control key={name} name={name} label={c(name)} value={content[name] ?? ''} onChange={value => change(name, value)} type={textarea ? 'textarea' : 'text'} help={name === 'story' ? c('storyHelp') : ''} error={fieldError(name)} />
  return <form className="catalog-form stack" onSubmit={submit} aria-busy={pending}>
    <div className="catalog-meta"><span className="status">{c(record.state)}</span><p>{record.event_id ? <a href={`/admin/auctions?edit=${encodeURIComponent(record.event_id)}`}>{c('assigned')}: {record.event_id}</a> : c('unassigned')}</p></div>
    {immutable && <p>{c('publishedNote')}</p>}{error && <ErrorPanel error={error} reload={reload} />}{saved && <p className="catalog-success" role="status"><Icon icon={CheckCircle2} /> {c('saved')}</p>}{dirty && <p>{c('unsaved')}</p>}
    <fieldset disabled={immutable || pending} className="stack"><legend>{c('identity')}</legend><div className="catalog-grid">{simple('title')}{simple('slug')}<Control name="category_id" label={c('category_id')} type="select" value={content.category_id ?? ''} onChange={value => change('category_id', value || null)} error={fieldError('category_id')}><option value="">—</option>{categories.map(category => <option key={category.id} value={category.id}>{lang === 'vi' ? category.name_vi : category.name_en}</option>)}</Control></div></fieldset>
    <fieldset disabled={immutable || pending} className="stack"><legend><Icon icon={BookOpen} /> {c('content')}</legend>{simple('description', true)}<div className="catalog-grid">{simple('brand')}{simple('era')}</div>{simple('condition', true)}{simple('story', true)}</fieldset>
    <fieldset disabled={immutable || pending} className="stack"><legend id="catalog-images">{c('gallery')}</legend><p>{c('mediaHelp')}</p><ol className="catalog-ordered">{content.images.map((image, index) => <li key={index} className="catalog-record stack-small"><div className="catalog-grid"><Control name={`images.${index}.path`} label={`${c('path')} ${index + 1}`} value={image.path} onChange={value => change('images', content.images.map((entry, i) => i === index ? { ...entry, path: value } : entry))} error={fieldError(`images.${index}.path`)} /><Control name={`images.${index}.alt`} label={`${c('alt')} ${index + 1}`} value={image.alt} onChange={value => change('images', content.images.map((entry, i) => i === index ? { ...entry, alt: value } : entry))} error={fieldError(`images.${index}.alt`)} /></div><div className="catalog-thumbnail"><LotImage src={image.path} alt={image.alt} /></div><MoveControls index={index} length={content.images.length} label={`${c('images')} ${index + 1}`} move={delta => change('images', moveItem(content.images, index, delta))} remove={() => change('images', content.images.filter((_, i) => i !== index))} /></li>)}</ol><Button type="button" variant="outline" onClick={() => change('images', [...content.images, { path: '', alt: '' }])}>{c('add')} · {c('images')}</Button></fieldset>
    <fieldset disabled={immutable || pending} className="stack"><legend>{c('specs')}</legend><ol className="catalog-ordered">{content.specifications.map((spec, index) => <li key={index} className="catalog-record stack-small"><div className="catalog-grid">{(['name', 'value'] as const).map(key => <Control key={key} name={`specifications.${index}.${key}`} label={`${c(key)} ${index + 1}`} value={spec[key]} onChange={value => change('specifications', content.specifications.map((entry, i) => i === index ? { ...entry, [key]: value } : entry))} error={fieldError(`specifications.${index}`)} />)}</div><MoveControls index={index} length={content.specifications.length} label={`${c('specifications')} ${index + 1}`} move={delta => change('specifications', moveItem(content.specifications, index, delta))} remove={() => change('specifications', content.specifications.filter((_, i) => i !== index))} /></li>)}</ol><Button type="button" variant="outline" onClick={() => change('specifications', [...content.specifications, { name: '', value: '' }])}>{c('add')} · {c('specifications')}</Button></fieldset>
    <fieldset disabled={immutable || pending} className="stack"><legend>{c('money')}</legend><div className="catalog-grid">{(['starting_price_minor', 'bid_increment_minor', 'shipping_fee_minor'] as const).map(key => <Control key={key} name={key === 'shipping_fee_minor' ? `fulfillment.${key}` : key} label={c(key)} value={prices[key]} onChange={value => { setPrices(old => ({ ...old, [key]: value })); setDirty(true); setSaved(false) }} help="USD · 0.00" error={fieldError(key === 'shipping_fee_minor' ? `fulfillment.${key}` : key)} />)}</div></fieldset>
    <fieldset disabled={immutable || pending} className="stack"><legend id="catalog-fulfillment">{c('fulfillment')}</legend>{(['shipping_enabled', 'pickup_enabled'] as const).map(key => <label key={key} className="catalog-checkbox"><input type="checkbox" checked={content.fulfillment[key]} onChange={event => change('fulfillment', { ...content.fulfillment, [key]: event.target.checked })} />{c(key)}</label>)}{(['shipping_instructions', 'pickup_instructions'] as const).map(key => <Control key={key} name={`fulfillment.${key}`} label={c(key)} value={content.fulfillment[key]} type="textarea" onChange={value => change('fulfillment', { ...content.fulfillment, [key]: value })} error={fieldError(`fulfillment.${key}`)} />)}</fieldset>
    <fieldset disabled={immutable || pending}><legend>{c('notes')}</legend>{simple('admin_notes', true)}</fieldset>
    {!immutable && <div className="catalog-sticky"><Button type="submit" disabled={pending || blocked}><Icon icon={Save} />{pending ? c('saving') : c('save')}</Button>{blocked && <span>{c('reloadRequired')}</span>}</div>}
  </form>
}

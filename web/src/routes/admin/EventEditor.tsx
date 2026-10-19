import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { LotImage } from '@/components/site/Presentation'
import { catalogAPI, CatalogError, eventContent, type AuctionEvent, type EventContent, type Lot, type PublicEvent } from '@/api/catalog'
import { useCatalogCopy } from '@/lib/catalog-copy'
import { localScheduleInput, parseLocalSchedule, parseInterval, scheduleText } from '@/lib/catalog-schedule'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { asCatalogError, moveItem, requiresReload } from '@/lib/catalog-form'
import { Control, ErrorPanel, MoveControls } from './Controls'

export default function EventEditor({ initial, lots, reload }: { initial: AuctionEvent; lots: Lot[]; reload: () => void }) {
  const { c, lang, errorText } = useCatalogCopy()
  const [record, setRecord] = useState(initial)
  const [content, setContent] = useState(() => eventContent(initial))
  const [timing, setTiming] = useState(() => ({ opens_at: localScheduleInput(initial.opens_at), first_closes_at: localScheduleInput(initial.first_closes_at), close_interval_ms: initial.close_interval_ms?.toString() ?? '' }))
  const [ids, setIDs] = useState(initial.lot_ids)
  const [candidate, setCandidate] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<CatalogError>()
  const [saved, setSaved] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [membersDirty, setMembersDirty] = useState(false)
  const [blocked, setBlocked] = useState(false)
  const [preview, setPreview] = useState<PublicEvent>()
  const [published, setPublished] = useState(false)
  const immutable = record.state === 'published'
  function change<K extends keyof EventContent>(key: K, value: EventContent[K]) { setContent(old => ({ ...old, [key]: value })); setDirty(true); setSaved(false); setPreview(undefined) }
  function changeIDs(values: string[]) { setIDs(values); setMembersDirty(true); setSaved(false); setPreview(undefined) }
  const fieldError = (name: string) => error?.fields[name] ? errorText(error.fields[name]) : undefined
  async function operation(action: () => Promise<void>) {
    if (pending || blocked) return
    setPending(true); setError(undefined); setSaved(false)
    try { await action() } catch (err) { const failure = asCatalogError(err); setError(failure); if (requiresReload(failure)) setBlocked(true) }
    finally { setPending(false) }
  }
  async function submit(event: FormEvent) {
    event.preventDefault()
    await operation(async () => {
      const fields: Record<string, string> = {}; const parsed: Record<string, number | null> = {}
      for (const [key, value] of Object.entries(timing)) { try { parsed[key] = key === 'close_interval_ms' ? parseInterval(value) : parseLocalSchedule(value) } catch { fields[key] = 'schedule' } }
      if (Object.keys(fields).length) throw new CatalogError('validation', 422, fields)
      const next = await catalogAPI.saveEvent(record, { ...content, opens_at: parsed.opens_at ?? null, first_closes_at: parsed.first_closes_at ?? null, close_interval_ms: parsed.close_interval_ms ?? null })
      setRecord(next); setContent(eventContent(next)); setDirty(false); setSaved(true); setPreview(undefined)
    })
  }
  const simple = (name: 'title' | 'slug' | 'theme' | 'description' | 'cover_path' | 'cover_alt' | 'admin_notes', textarea = false) => <Control key={name} name={name} label={c(name)} value={content[name]} onChange={value => change(name, value)} type={textarea ? 'textarea' : 'text'} error={fieldError(name)} />
  const available = lots.filter(lot => lot.state === 'draft' && (!lot.event_id || lot.event_id === record.id) && !ids.includes(lot.id))
  return <div className="catalog-form stack" aria-busy={pending}>
    <div className="catalog-meta"><span className="status">{c(record.state)}</span></div>
    {immutable && <p>{c('publishedNote')}</p>}{error && <ErrorPanel error={error} reload={reload} />}{saved && <p className="catalog-success" role="status">{c('saved')}</p>}{published && <p className="catalog-success" role="status">{c('publishedSuccess')}</p>}{(dirty || membersDirty) && <p>{c('unsaved')}</p>}
    <form className="stack" onSubmit={submit}><fieldset disabled={immutable || pending} className="stack"><legend>{c('content')}</legend><div className="catalog-grid">{simple('title')}{simple('slug')}</div>{simple('theme')}{simple('description', true)}<p>{c('mediaHelp')}</p><div className="catalog-grid">{simple('cover_path')}{simple('cover_alt')}</div><div className="catalog-cover"><LotImage src={content.cover_path} alt={content.cover_alt} /></div>
      {simple('admin_notes', true)}</fieldset>
      <fieldset disabled={immutable || pending} className="stack"><legend>{c('schedule')}</legend><p>{c('timezone')}: <strong>{Intl.DateTimeFormat().resolvedOptions().timeZone}</strong></p><p>{c('scheduleNote')}</p><div className="catalog-grid">{(['opens_at', 'first_closes_at', 'close_interval_ms'] as const).map(key => <Control key={key} name={key} label={c(key)} type={key === 'close_interval_ms' ? 'text' : 'datetime-local'} value={timing[key]} onChange={value => { setTiming(old => ({ ...old, [key]: value })); setDirty(true); setSaved(false); setPreview(undefined) }} error={fieldError(key)} />)}</div></fieldset>
      {!immutable && <Button type="submit" disabled={pending || blocked}>{pending ? c('saving') : c('save')}</Button>}
    </form>
    <section className="stack" id="catalog-lot_ids"><h2>{c('membership')}</h2><fieldset disabled={immutable || pending || blocked} className="stack"><legend className="sr-only">{c('membership')}</legend><ol className="catalog-ordered">{ids.map((id, index) => {
      const lot = lots.find(value => value.id === id)
      const slot = record.schedule.find(value => value.lot_id === id)
      return <li key={id} className="catalog-record stack-small"><h3><a href={`/admin/lots?edit=${encodeURIComponent(id)}`}>{lot?.title ?? id}</a></h3><p>{c('starting_price_minor')}: {formatCatalogMoney(lot?.starting_price_minor ?? null)}</p>{slot && <p className="numeric">{c('opens_at')}: {scheduleText(slot.opens_at, lang)}<br />{c('first_closes_at')}: {scheduleText(slot.closes_at, lang)}</p>}<MoveControls index={index} length={ids.length} label={lot?.title ?? id} move={delta => changeIDs(moveItem(ids, index, delta))} remove={() => changeIDs(ids.filter(value => value !== id))} /></li>
    })}</ol>
    {!immutable && <><Control name="available" label={c('available')} type="select" value={candidate} onChange={setCandidate}><option value="">—</option>{available.map(lot => <option key={lot.id} value={lot.id}>{lot.title}</option>)}</Control><div className="catalog-actions"><Button type="button" variant="outline" disabled={!candidate} onClick={() => { changeIDs([...ids, candidate]); setCandidate('') }}>{c('add')}</Button><Button type="button" disabled={!membersDirty} onClick={() => operation(async () => { const next = await catalogAPI.members(record, ids); setRecord(next); setMembersDirty(false); setSaved(true); setPreview(undefined) })}>{c('saveMembership')}</Button></div></>}
    </fieldset></section>
    <section className="stack"><h2>{c('preview')}</h2><p>{c('previewNote')}</p><div className="catalog-actions"><Button type="button" variant="outline" disabled={pending || blocked || dirty || membersDirty} onClick={() => operation(async () => { setPreview(await catalogAPI.preview(record.id)) })}>{c('preview')}</Button>
      {!immutable && <Button type="button" disabled={pending || blocked || dirty || membersDirty || !preview} onClick={() => operation(async () => { const result = await catalogAPI.publish(record); setPreview(result); setRecord(old => ({ ...old, state: 'published' })); setPublished(true) })}>{c('publish')}</Button>}
    </div>{preview && <article className="catalog-preview stack"><h3>{preview.title}</h3><p>{preview.theme}</p><p>{preview.description}</p><div className="catalog-cover"><LotImage src={preview.cover_path} alt={preview.cover_alt} /></div><p>{c('planned_end')}: {scheduleText(preview.planned_end, lang)}</p><ol className="catalog-ordered">{preview.lots.map(lot => <li key={lot.id} className="catalog-record stack-small"><h3>{lot.title}</h3><p>{lot.description}</p><p>{c('condition')}: {lot.condition}</p><p>{c('category_id')}: {lot.category_id ?? '—'}</p>{lot.brand && <p>{c('brand')}: {lot.brand}</p>}{lot.era && <p>{c('era')}: {lot.era}</p>}{lot.story && <div><p>{c('storyHelp')}</p><p>{lot.story}</p></div>}<div className="catalog-preview-gallery">{lot.images.map((image, index) => <LotImage key={index} src={image.path} alt={image.alt} />)}</div><dl>{lot.specifications.map((spec, index) => <div key={index}><dt>{spec.name}</dt><dd>{spec.value}</dd></div>)}</dl><p>{c('starting_price_minor')}: {formatCatalogMoney(lot.starting_price_minor)} · {c('bid_increment_minor')}: {formatCatalogMoney(lot.bid_increment_minor)}</p><p>{c('opens_at')}: {scheduleText(lot.opens_at, lang)}<br />{c('first_closes_at')}: {scheduleText(lot.closes_at, lang)}</p><p>{lot.fulfillment.shipping_instructions} {formatCatalogMoney(lot.fulfillment.shipping_fee_minor)}</p><p>{lot.fulfillment.pickup_instructions}</p></li>)}</ol></article>}</section>
    {blocked && <p>{c('reloadRequired')}</p>}
  </div>
}

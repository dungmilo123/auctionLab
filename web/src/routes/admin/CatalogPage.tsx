import { useEffect, useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Plus } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { LotImage } from '@/components/site/Presentation'
import { PageHeading } from '@/components/site/Presentation'
import { catalogAPI, CatalogError, type AuctionEvent, type Category, type Lot } from '@/api/catalog'
import { catalogSelection } from '@/lib/catalog-selection'
import { useCatalogCopy } from '@/lib/catalog-copy'
import { Control, ErrorPanel } from './Controls'
import { asCatalogError } from '@/lib/catalog-form'
import LotEditor from './LotEditor'
import EventEditor from './EventEditor'

type Loaded = { lots: Lot[]; events: AuctionEvent[]; categories: Category[] }
export default function CatalogPage({ kind }: { kind: 'lots' | 'events' }) {
  const { c } = useCatalogCopy()
  const selection = catalogSelection(window.location.search)
  const base = kind === 'lots' ? '/admin/lots' : '/admin/auctions'
  const [loaded, setLoaded] = useState<Loaded>()
  const [error, setError] = useState<CatalogError>()
  const [reload, setReload] = useState(0)
  useEffect(() => {
    let current = true
    Promise.all([catalogAPI.lots(), catalogAPI.events(), catalogAPI.categories()])
      .then(([lots, events, categories]) => { if (current) setLoaded({ lots, events, categories }) })
      .catch(err => { if (current) setError(asCatalogError(err)) })
    return () => { current = false }
  }, [kind, reload])
  const refresh = () => { setLoaded(undefined); setError(undefined); setReload(value => value + 1) }
  return <><PageHeading title={c(kind)} intro={c('notice')} />
    {selection.kind !== 'list' && <a href={base}>{c('back')}</a>}
    {error ? <ErrorPanel error={error} reload={refresh} /> : !loaded ? <p role="status">{c('loading')}</p>
      : selection.kind === 'invalid' ? <ErrorPanel error={new CatalogError('not_found')} />
      : selection.kind === 'new' ? <NewRecord kind={kind} base={base} />
      : selection.kind === 'edit' ? kind === 'lots'
        ? loaded.lots.find(record => record.id === selection.id) ? <LotEditor key={`${selection.id}-${reload}`} initial={loaded.lots.find(record => record.id === selection.id)!} categories={loaded.categories} reload={refresh} /> : <ErrorPanel error={new CatalogError('not_found')} />
        : loaded.events.find(record => record.id === selection.id) ? <EventEditor key={`${selection.id}-${reload}`} initial={loaded.events.find(record => record.id === selection.id)!} lots={loaded.lots} reload={refresh} /> : <ErrorPanel error={new CatalogError('not_found')} />
      : <><a className="site-button site-button-primary" href={`${base}?new=1`}><Icon icon={Plus} />{c(kind === 'lots' ? 'newLot' : 'newEvent')}</a>
        {(kind === 'lots' ? loaded.lots : loaded.events).length === 0 ? <p>{c('empty')}</p> : <ul className="catalog-records">{(kind === 'lots' ? loaded.lots : loaded.events).map(record => <li key={record.id} className="catalog-record catalog-list-row"><div className="catalog-list-thumbnail"><LotImage src={'images' in record ? record.images[0]?.path : record.cover_path} alt={'images' in record ? record.images[0]?.alt ?? '' : record.cover_alt} /></div><div className="stack-small"><h2><a href={`${base}?edit=${encodeURIComponent(record.id)}`}>{record.title}</a></h2><p><span className="status">{c(record.state)}</span> {'event_id' in record && <span>{record.event_id ? `${c('assigned')}: ${record.event_id}` : c('unassigned')}</span>}</p><p className="muted">{record.slug}</p></div></li>)}</ul>}</>}
  </>
}
function NewRecord({ kind, base }: { kind: 'lots' | 'events'; base: string }) {
  const { c, errorText } = useCatalogCopy()
  const [title, setTitle] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<CatalogError>()
  async function submit(event: FormEvent) {
    event.preventDefault(); setPending(true); setError(undefined)
    try { const record = kind === 'lots' ? await catalogAPI.createLot(title) : await catalogAPI.createEvent(title); window.location.assign(`${base}?edit=${encodeURIComponent(record.id)}`) }
    catch (err) { setError(asCatalogError(err)); setPending(false) }
  }
  return <form className="stack" onSubmit={submit} aria-busy={pending}>
    {error && <ErrorPanel error={error} reload={() => window.location.reload()} />}
    <Control name="title" label={c('title')} value={title} onChange={setTitle} help={c('optionalHelp')} error={error?.fields.title ? errorText(error.fields.title) : undefined} />
    <Button type="submit" disabled={pending || !title.trim() || error?.code === 'commit_unknown'}>{pending ? c('saving') : c('create')}</Button>
  </form>
}

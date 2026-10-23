import { useState } from 'react'
import { Dialog } from 'radix-ui'
import { ArrowLeft, ArrowRight, Clock, Maximize2, X, PackageSearch, CircleAlert } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import WatchControl from '@/routes/account/WatchControl'
import { displayLot, displayEvent } from '@/lib/seed-display'
import type {
  Category,
  PublicEvent,
  PublicLot,
  CatalogError,
} from '@/api/catalog'
import { LotImage } from '@/components/site/Presentation'
import { phase } from '@/lib/discovery'
import { useDiscoveryCopy, type DiscoveryKey } from '@/lib/discovery-copy'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { MAX_CATALOG_INSTANT, scheduleText } from '@/lib/catalog-schedule'
import { lotURL, eventURL } from '@/lib/discovery-links'
export function CategoryName({
  id,
  categories,
}: {
  id: string | null
  categories: Category[]
}) {
  const { lang, d } = useDiscoveryCopy()
  const c = categories.find((c) => c.id === id)
  return <>{c ? (lang === 'vi' ? c.name_vi : c.name_en) : d('unknown')}</>
}
export function PlannedBadge({
  open,
  close,
  now,
}: {
  open: number | null
  close: number | null
  now: number
}) {
  const { d } = useDiscoveryCopy()
  const p = phase(open, close, now)
  return <span className={`status planned-${p}`}>{d(p)}</span>
}
export function Times({
  open,
  close,
  event = false,
}: {
  open: number | null
  close: number | null
  event?: boolean
}) {
  const { lang, d } = useDiscoveryCopy()
  const time = (v: number | null) =>
    v === null || !Number.isSafeInteger(v) || v < 0 || v > MAX_CATALOG_INSTANT
      ? d('unknown')
      : scheduleText(v, lang)
  return (
    <div className="stack-small numeric">
      <p>
        {d('opens')}: {time(open)}
      </p>
      <p>
        {d(event ? 'end' : 'closes')}: {time(close)}
      </p>
      <p className="muted">
        {d('timezone')} · {Intl.DateTimeFormat().resolvedOptions().timeZone}
      </p>
    </div>
  )
}
export function PublicLotCard({
  lot: rawLot,
  categories,
  now,
  showCategory = true,
}: {
  lot: PublicLot
  categories: Category[]
  now: number
  showCategory?: boolean
}) {
  const { d, lang } = useDiscoveryCopy()
  const lot = displayLot(rawLot)
  return (
    <article className="lot-card">
      <a className="card-image-link" href={lotURL(lot.slug)} tabIndex={-1} aria-hidden="true"><LotImage src={lot.images[0]?.path} alt={lot.images[0]?.alt ?? ''} /></a>
      <WatchControl lot={rawLot} target={lotURL(lot.slug)} compact />
      <div className="lot-body stack-small">
        <h3>
          <a href={lotURL(lot.slug)}>{lot.title}</a>
        </h3>
        {showCategory && <p className="muted">
          <CategoryName id={lot.category_id} categories={categories} />
        </p>}
        <PlannedBadge open={lot.opens_at} close={lot.closes_at} now={now} />
        <p className="numeric">
          {d('opening')}: {formatCatalogMoney(lot.starting_price_minor)}
        </p>
        <p className="card-time numeric"><Icon icon={Clock} size={16} /><span>{d('closes')}: {lot.closes_at === null ? d('unknown') : scheduleText(lot.closes_at, lang)}</span></p>
      </div>
    </article>
  )
}
export function EventCard({ event: rawEvent, now }: { event: PublicEvent; now: number }) {
  const event = displayEvent(rawEvent), { lang, d } = useDiscoveryCopy()
  return (
    <article className="lot-card event-card">
      <a className="card-image-link" href={eventURL(event.slug)} tabIndex={-1} aria-hidden="true"><LotImage src={event.cover_path} alt={event.cover_alt} /></a>
      <div className="lot-body stack-small">
        <h3>
          <a href={eventURL(event.slug)}>{event.title}</a>
        </h3>
        <p>{event.theme}</p>
        <PlannedBadge
          open={event.opens_at}
          close={event.planned_end}
          now={now}
        />
        <p className="card-time numeric"><Icon icon={Clock} size={16} /><span>{d('opens')}: {event.opens_at === null ? d('unknown') : scheduleText(event.opens_at, lang)}</span></p>
      </div>
    </article>
  )
}
export function DataState({
  kind,
  error,
  retry,
  index = '/lots',
}: {
  kind: 'loading' | 'empty' | 'noResults' | 'error'
  error?: CatalogError
  retry?: () => void
  index?: string
}) {
  const { d } = useDiscoveryCopy()
  const title: DiscoveryKey =
    kind === 'error'
      ? error?.code === 'not_found'
        ? 'missing'
        : error?.code === 'unavailable'
          ? 'offline'
          : 'error'
      : kind
  return (
    <section
      className="empty-state stack"
      role={kind === 'loading' ? 'status' : undefined}
    >
      <Icon icon={kind === 'error' ? CircleAlert : PackageSearch} size={28} />
      <h2>{d(title)}</h2>
      {kind !== 'loading' && (
        <>
          <p>
            {d(
              kind === 'error'
                ? error?.code === 'not_found'
                  ? 'missingHelp'
                  : 'errorHelp'
                : kind === 'empty'
                  ? 'emptyHelp'
                  : 'noResultsHelp',
            )}
          </p>
          <div className="actions">
            {retry && error?.code !== 'not_found' && (
              <button
                className="site-button site-button-outline"
                onClick={retry}
              >
                {d('retry')}
              </button>
            )}
            <a className="site-button site-button-link" href={index}>
              {d(
                kind === 'noResults'
                  ? 'clear'
                  : index === '/auctions'
                    ? 'allEvents'
                    : 'allLots',
              )}
            </a>
          </div>
        </>
      )}
    </section>
  )
}
export function Gallery({ images }: { images: PublicLot['images'] }) {
  const { d, lang } = useDiscoveryCopy()
  const [selected, setSelected] = useState(0)
  const current = images[selected] ?? images[0]
  const text = { inspect: lang === 'vi' ? 'Xem ảnh lớn' : 'Inspect image', close: lang === 'vi' ? 'Đóng' : 'Close', previous: lang === 'vi' ? 'Ảnh trước' : 'Previous image', next: lang === 'vi' ? 'Ảnh tiếp' : 'Next image' }
  const move = (offset: number) => setSelected(i => (i + offset + images.length) % images.length)
  return <section className="stack-small gallery" aria-label={d('gallery')}>
    <Dialog.Root>
      <Dialog.Trigger asChild><button className="gallery-open" type="button" aria-label={text.inspect}><LotImage src={current?.path} alt={current?.alt ?? ''} eager /><span className="inspect-label"><Icon icon={Maximize2} size={18} />{text.inspect}</span></button></Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="website-overlay inspection-backdrop" />
        <Dialog.Content className="website-overlay inspection-dialog" onKeyDown={e => { if (images.length > 1 && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) { e.preventDefault(); move(e.key === 'ArrowLeft' ? -1 : 1) } }}>
          <div className="inspection-toolbar"><Dialog.Title>{d('gallery')}</Dialog.Title><Dialog.Close className="site-button site-button-outline"><Icon icon={X} />{text.close}</Dialog.Close></div>
          <Dialog.Description className="muted">{d('image')} {selected + 1} / {images.length || 1}</Dialog.Description>
          <LotImage src={current?.path} alt={current?.alt ?? ''} eager />
          {images.length > 1 && <div className="inspection-toolbar"><button type="button" className="site-button site-button-outline" onClick={() => move(-1)}><Icon icon={ArrowLeft} />{text.previous}</button><span className="numeric" role="status">{selected + 1} / {images.length}</span><button type="button" className="site-button site-button-outline" onClick={() => move(1)}>{text.next}<Icon icon={ArrowRight} /></button></div>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
    <div className="gallery-thumbnails">{images.map((image, i) => <button key={`${i}-${image.path}`} type="button" aria-label={`${d('image')} ${i + 1}: ${image.alt}`} aria-pressed={i === selected} onClick={() => setSelected(i)}><LotImage src={image.path} alt={image.alt} /></button>)}</div>
  </section>
}

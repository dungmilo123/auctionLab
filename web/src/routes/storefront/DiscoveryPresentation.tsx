import { useState } from 'react'
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
export function Demo({ demo }: { demo: boolean }) {
  const { d } = useDiscoveryCopy()
  return demo ? <span className="demo-marker">{d('demo')}</span> : null
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
  lot,
  categories,
  now,
}: {
  lot: PublicLot
  categories: Category[]
  now: number
}) {
  const { d } = useDiscoveryCopy()
  return (
    <article className="lot-card">
      <LotImage src={lot.images[0]?.path} alt={lot.images[0]?.alt ?? ''} />
      <div className="lot-body stack-small">
        <Demo demo={lot.demo} />
        <h3>
          <a href={lotURL(lot.slug)}>{lot.title}</a>
        </h3>
        <p className="muted">
          <CategoryName id={lot.category_id} categories={categories} />
        </p>
        <PlannedBadge open={lot.opens_at} close={lot.closes_at} now={now} />
        <p className="numeric">
          {d('opening')}: {formatCatalogMoney(lot.starting_price_minor)}
        </p>
        <Times open={lot.opens_at} close={lot.closes_at} />
      </div>
    </article>
  )
}
export function EventCard({ event, now }: { event: PublicEvent; now: number }) {
  return (
    <article className="lot-card">
      <LotImage src={event.cover_path} alt={event.cover_alt} />
      <div className="lot-body stack-small">
        <Demo demo={event.demo} />
        <h3>
          <a href={eventURL(event.slug)}>{event.title}</a>
        </h3>
        <p>{event.theme}</p>
        <PlannedBadge
          open={event.opens_at}
          close={event.planned_end}
          now={now}
        />
        <Times open={event.opens_at} close={event.planned_end} event />
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
  const { d } = useDiscoveryCopy()
  const [selected, setSelected] = useState(0)
  const current = images[selected]
  return (
    <section className="stack-small" aria-label={d('gallery')}>
      <LotImage src={current?.path} alt={current?.alt ?? ''} />
      <div className="gallery-thumbnails">
        {images.map((image, i) => (
          <button
            key={`${i}-${image.path}`}
            type="button"
            aria-label={`${d('image')} ${i + 1}: ${image.alt}`}
            aria-pressed={i === selected}
            onClick={() => setSelected(i)}
          >
            <LotImage src={image.path} alt={image.alt} />
          </button>
        ))}
      </div>
    </section>
  )
}

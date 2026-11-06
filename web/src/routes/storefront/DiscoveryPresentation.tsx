import { useState, type ReactNode } from 'react'
import { Dialog } from 'radix-ui'
import { ArrowLeft, ArrowRight, Clock, Maximize2, X, PackageSearch, CircleAlert } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Countdown, Note, NoteCode, NotePrice, NoteStatus } from '@/components/site/Note'
import WatchControl from '@/routes/account/WatchControl'
import { displayLot, displayEvent } from '@/lib/seed-display'
import type {
  Category,
  PublicEvent,
  PublicLot,
  CatalogError,
} from '@/api/catalog'
import { LotImage } from '@/components/site/Presentation'
import { useSiteCopy } from '@/lib/site-copy'
import { phase, noteTone } from '@/lib/discovery'
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
  const tone = p === 'window' ? 'live' : p === 'upcoming' ? 'upcoming' : 'ended'
  return <Badge tone={tone}>{d(p)}</Badge>
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
    <div className="flex flex-col gap-2.5 font-mono text-sm numeric">
      <p>
        {d('opens')}: {time(open)}
      </p>
      <p>
        {d(event ? 'end' : 'closes')}: {time(close)}
      </p>
      <p className="text-muted-foreground">
        {d('timezone')} · {Intl.DateTimeFormat().resolvedOptions().timeZone}
      </p>
    </div>
  )
}
export function LotGrid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-1 gap-7 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
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
  const { d } = useDiscoveryCopy()
  const lot = displayLot(rawLot)
  return (
    <Note tone={noteTone(lot.opens_at, lot.closes_at, now)} className="px-3 pt-3 pb-4.5">
      <a className="block" href={lotURL(lot.slug)} tabIndex={-1} aria-hidden="true"><LotImage src={lot.images[0]?.path} alt={lot.images[0]?.alt ?? ''} /></a>
      <WatchControl lot={rawLot} target={lotURL(lot.slug)} compact />
      <div className="flex flex-col gap-2.5 px-1">
        <NoteCode left={showCategory ? <CategoryName id={lot.category_id} categories={categories} /> : null} right={lot.era} />
        <h3 className="text-[1.0625rem] leading-snug font-medium tracking-[-0.015em]">
          <a className="inline-flex min-h-11 items-center text-ink no-underline hover:underline hover:underline-offset-4" href={lotURL(lot.slug)}>{lot.title}</a>
        </h3>
        <NoteStatus>
          <PlannedBadge open={lot.opens_at} close={lot.closes_at} now={now} />
          <Countdown close={lot.closes_at} now={now} />
        </NoteStatus>
        <NotePrice label={d('opening')}>{formatCatalogMoney(lot.starting_price_minor)}</NotePrice>
      </div>
    </Note>
  )
}
export function EventCard({ event: rawEvent, now }: { event: PublicEvent; now: number }) {
  const event = displayEvent(rawEvent), { lang, d } = useDiscoveryCopy()
  const { t } = useSiteCopy()
  return (
    <Note tone={noteTone(event.opens_at, event.planned_end, now)} className="px-3 pt-3 pb-4.5">
      <a className="block" href={eventURL(event.slug)} tabIndex={-1} aria-hidden="true"><LotImage src={event.cover_path} alt={event.cover_alt} /></a>
      <div className="flex flex-col gap-2.5 px-1">
        <NoteCode left={t('auctionDetail')} right={event.theme} />
        <h3 className="text-[1.0625rem] leading-snug font-medium tracking-[-0.015em]">
          <a className="inline-flex min-h-11 items-center text-ink no-underline hover:underline hover:underline-offset-4" href={eventURL(event.slug)}>{event.title}</a>
        </h3>
        <NoteStatus>
          <PlannedBadge
            open={event.opens_at}
            close={event.planned_end}
            now={now}
          />
          <Countdown close={event.planned_end} now={now} />
        </NoteStatus>
        <p className="flex items-baseline gap-2 text-[0.8125rem] text-muted-foreground numeric"><Icon icon={Clock} size={14} /><span>{d('opens')}: {event.opens_at === null ? d('unknown') : scheduleText(event.opens_at, lang)}</span></p>
      </div>
    </Note>
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
      className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8"
      role={kind === 'loading' ? 'status' : undefined}
    >
      <Icon icon={kind === 'error' ? CircleAlert : PackageSearch} size={28} />
      <h2>{d(title)}</h2>
      {kind !== 'loading' && (
        <>
          <p className="text-muted-foreground">
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
          <div className="flex flex-wrap items-start gap-3">
            {retry && error?.code !== 'not_found' && (
              <Button type="button" variant="outline" onClick={retry}>
                {d('retry')}
              </Button>
            )}
            <Button asChild variant="link">
              <a href={index}>
                {d(
                  kind === 'noResults'
                    ? 'clear'
                    : index === '/auctions'
                      ? 'allEvents'
                      : 'allLots',
                )}
              </a>
            </Button>
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
  return <section className="flex flex-col gap-2.5" aria-label={d('gallery')}>
    <Dialog.Root>
      <Dialog.Trigger asChild><button className="w-full cursor-zoom-in border-0 bg-transparent p-0 text-start" type="button" aria-label={text.inspect}><LotImage src={current?.path} alt={current?.alt ?? ''} eager /><span className="flex min-h-11 items-center gap-2 font-mono text-[0.6875rem] tracking-[0.12em] uppercase text-ink"><Icon icon={Maximize2} size={18} />{text.inspect}</span></button></Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[80] animate-[panel-in_240ms_cubic-bezier(.22,1,.36,1)] bg-[#111111D9]" />
        <Dialog.Content className="fixed inset-6 z-[81] m-auto flex max-h-[calc(100dvh-48px)] max-w-[1100px] animate-[panel-in_240ms_cubic-bezier(.22,1,.36,1)] flex-col gap-4 overflow-auto rounded-[2px] bg-background p-6 font-sans" onKeyDown={e => { if (images.length > 1 && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) { e.preventDefault(); move(e.key === 'ArrowLeft' ? -1 : 1) } }}>
          <div className="flex flex-wrap items-center justify-between gap-3"><Dialog.Title className="font-display text-xl font-medium tracking-[-0.02em]">{d('gallery')}</Dialog.Title><Dialog.Close asChild><Button variant="outline"><Icon icon={X} />{text.close}</Button></Dialog.Close></div>
          <Dialog.Description className="text-muted-foreground">{d('image')} {selected + 1} / {images.length || 1}</Dialog.Description>
          <LotImage src={current?.path} alt={current?.alt ?? ''} eager plain />
          {images.length > 1 && <div className="flex flex-wrap items-center justify-between gap-3"><Button type="button" variant="outline" onClick={() => move(-1)}><Icon icon={ArrowLeft} />{text.previous}</Button><span className="font-mono numeric" role="status">{selected + 1} / {images.length}</span><Button type="button" variant="outline" onClick={() => move(1)}>{text.next}<Icon icon={ArrowRight} /></Button></div>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
    <div className="flex flex-wrap gap-3">{images.map((image, i) => <button key={`${i}-${image.path}`} type="button" aria-label={`${d('image')} ${i + 1}: ${image.alt}`} aria-pressed={i === selected} onClick={() => setSelected(i)} className={`w-[88px] shrink-0 cursor-pointer rounded-[2px] border-2 bg-transparent p-1 ${i === selected ? 'border-ink' : 'border-ink/20'}`}><LotImage src={image.path} alt={image.alt} /></button>)}</div>
  </section>
}

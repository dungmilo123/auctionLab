import RecommendationSection from './RecommendationSection'
import * as m from 'motion/react-m'
import { ArrowUpRight } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { LotImage } from '@/components/site/Presentation'
import { Note, NoteCode, NoteMagnet, NotePrice, NoteStatus, Countdown, type NoteTone } from '@/components/site/Note'
import { Button } from '@/components/ui/button'
import { useWebsiteReducedMotion, motionEase } from '@/components/site/motion'
import { displayEvent, displayLot } from '@/lib/seed-display'
import { eventURL, lotURL } from '@/lib/discovery-links'
import { useSiteCopy } from '@/lib/site-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { homeSelection, noteTone } from '@/lib/discovery'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { scheduleText } from '@/lib/catalog-schedule'
import { usePublicPage } from './usePublicPage'
import { loadHome } from './loads'
import {
  DataState,
  EventCard,
  LotGrid,
  PlannedBadge,
  PublicLotCard,
} from './DiscoveryPresentation'

type Slot = 'lead' | 'a' | 'b' | 'c'
type Pin = {
  tone: NoteTone
  code: string
  detail: string
  title: string
  href: string
  image?: string
  alt: string
  open: number | null
  close: number | null
  price?: string
  time: string
}

// Each note sizes to its own content; the board stacks them in two columns
// (lead + c on the left, a + b on the right) so no note stretches to match a neighbour.
const slotClass: Record<Slot, string> = {
  lead: 'w-full',
  a: 'w-full',
  b: 'w-full',
  c: 'w-[78%] self-end max-[900px]:w-full max-[900px]:self-auto',
}

const slotTilt: Record<Slot, string> = {
  lead: 'rotate-[-1.1deg]',
  a: 'rotate-[1.5deg]',
  b: 'rotate-[-0.7deg]',
  c: 'rotate-[1.9deg]',
}

function PinnedNote({
  pin,
  slot,
  delay,
  now,
}: {
  pin: Pin
  slot: Slot
  delay: number
  now: number
}) {
  const { d } = useDiscoveryCopy()
  const reduce = useWebsiteReducedMotion()
  const lead = slot === 'lead'
  return (
    <m.div
      className={`relative z-[1] min-w-0 ${slotClass[slot]}`}
      initial={{
        opacity: reduce ? 1 : 0,
        y: reduce ? 0 : -26,
        rotate: reduce ? 0 : -3,
      }}
      animate={{ opacity: 1, y: 0, rotate: 0 }}
      transition={
        reduce
          ? { duration: 0 }
          : { type: 'spring', stiffness: 190, damping: 22, mass: 0.9, delay }
      }
    >
      <Note tone={pin.tone} className={slotTilt[slot]}>
        <NoteMagnet />
        <NoteCode left={pin.code} right={pin.detail} />
        <h3 className={lead ? 'text-[clamp(1.3rem,1.9vw,1.65rem)] leading-[1.1] font-medium tracking-[-0.03em]' : 'text-base leading-snug font-medium'}>
          <a href={pin.href}>{pin.title}</a>
        </h3>
        {pin.image !== undefined && (
          <a className="block" href={pin.href} tabIndex={-1} aria-hidden="true">
            <LotImage src={pin.image} alt={pin.alt} eager={lead} />
          </a>
        )}
        <NoteStatus>
          <PlannedBadge open={pin.open} close={pin.close} now={now} />
          <Countdown close={pin.close} now={now} />
        </NoteStatus>
        {pin.price && <NotePrice label={d('opening')} className="mt-auto">{pin.price}</NotePrice>}
        <p className="flex items-baseline gap-2 text-[0.8125rem] text-muted-foreground numeric">{pin.time}</p>
      </Note>
    </m.div>
  )
}

export default function HomePage() {
  const { t } = useSiteCopy()
  const { lang, d } = useDiscoveryCopy()
  const { state, retry, now } = usePublicPage('home', loadHome)
  const selection =
    state.status === 'success'
      ? homeSelection(state.response.data.record, now)
      : null
  const categories =
    state.status === 'success' ? state.response.data.categories : []
  const reduce = useWebsiteReducedMotion()
  const lead = selection?.featured[0] ? displayEvent(selection.featured[0]) : null
  const lots = selection ? selection.nearest.slice(0, 3).map(displayLot) : []
  const entrance = state.status === 'success' && !reduce
  const enter = (delay: number) => ({
    initial: { opacity: entrance ? 0 : 1, y: entrance ? 12 : 0 },
    animate: { opacity: 1, y: 0 },
    transition: { duration: entrance ? 0.5 : 0, delay: entrance ? delay : 0, ease: motionEase },
  })
  const categoryName = (id: string | null) => {
    const c = categories.find((c) => c.id === id)
    return c ? (lang === 'vi' ? c.name_vi : c.name_en) : d('unknown')
  }
  const leadPin: Pin | null = lead
    ? {
        tone: noteTone(lead.opens_at, lead.planned_end, now),
        code: t('auctionDetail'),
        detail: lead.theme,
        title: lead.title,
        href: eventURL(lead.slug),
        image: lead.cover_path,
        alt: lead.cover_alt,
        open: lead.opens_at,
        close: lead.planned_end,
        time: `${d('opens')}: ${
          lead.opens_at === null ? d('unknown') : scheduleText(lead.opens_at, lang)
        }`,
      }
    : null
  const lotPins: Pin[] = lots.map((lot) => ({
    tone: noteTone(lot.opens_at, lot.closes_at, now),
    code: categoryName(lot.category_id),
    detail: lot.era ?? '',
    title: lot.title,
    href: lotURL(lot.slug),
    image: lot.images[0]?.path,
    alt: lot.images[0]?.alt ?? '',
    open: lot.opens_at,
    close: lot.closes_at,
    price: formatCatalogMoney(lot.starting_price_minor, lang),
    time: `${d('closes')}: ${
      lot.closes_at === null ? d('unknown') : scheduleText(lot.closes_at, lang)
    }`,
  }))
  const clock = new Date(now)
  return (
    <div className="flex flex-col gap-20">
      <section
        className="grid items-start gap-10 pb-18 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:gap-14"
        key={state.status === 'success' ? 'published' : 'loading'}
      >
        <m.div className="flex flex-col gap-2.5" {...enter(0)}>
          <p className="mono-label tracking-[0.16em] uppercase">{t('concept')}</p>
          <h1 className="max-w-[11ch] font-display text-[clamp(2.6rem,6vw,5rem)] leading-[0.94] font-medium tracking-[-0.05em]">{lead?.title ?? t('tagline')}</h1>
          <p className="max-w-[34rem] text-base leading-relaxed text-muted-foreground">{lead?.description ?? t('intro')}</p>
          <m.div className="flex flex-wrap items-start gap-3 pt-2" {...enter(0.12)}>
            <Button asChild size="lg">
              <a href={lead ? eventURL(lead.slug) : '/lots'}>
                {lead ? d('parent') : d('allLots')}
                <Icon icon={ArrowUpRight} />
              </a>
            </Button>
            <Button asChild variant="outline" size="lg">
              <a href={lead ? '/lots' : '/auctions'}>
                {lead ? d('allLots') : d('allEvents')}
              </a>
            </Button>
          </m.div>
        </m.div>
        <div className="min-w-0">
          <div className="mb-4 flex items-baseline justify-between gap-4 border-b border-ink pb-2.5">
            <span className="mono-label">{d('published')}</span>
            <time
              className="mono-label numeric"
              dateTime={clock.toISOString()}
              aria-hidden="true"
            >
              {clock.toLocaleTimeString(lang === 'vi' ? 'vi-VN' : 'en-US', {
                hour12: false,
              })}
            </time>
          </div>
          <div className="board-surface grid min-h-[520px] grid-cols-12 items-start gap-5 px-[22px] py-[26px] max-[900px]:min-h-0 max-[900px]:grid-cols-1 max-[900px]:gap-5 max-[900px]:px-4 max-[900px]:py-6">
            {leadPin ? (
              <>
                <div className="col-span-7 flex flex-col items-start gap-6 max-[900px]:col-span-full max-[900px]:gap-5">
                  <PinnedNote pin={leadPin} slot="lead" delay={0.06} now={now} />
                  {lotPins[2] && <PinnedNote key={lotPins[2].href} pin={lotPins[2]} slot="c" delay={0.32} now={now} />}
                </div>
                <div className="col-span-5 flex flex-col gap-6 pt-8 max-[900px]:col-span-full max-[900px]:gap-5 max-[900px]:pt-0">
                  {lotPins.slice(0, 2).map((pin, i) => (
                    <PinnedNote key={pin.href} pin={pin} slot={i === 0 ? 'a' : 'b'} delay={0.16 + i * 0.08} now={now} />
                  ))}
                </div>
              </>
            ) : (
              <div className="col-span-full grid min-h-[520px] place-items-center p-8 text-center max-[900px]:min-h-[320px]">
                <p className="mono-label">
                  {state.status === 'loading' ? d('loading') : d('empty')}
                </p>
              </div>
            )}
          </div>
          <p className="mono-label mt-3 flex justify-between gap-4">
            <span>{d('notice')}</span>
            <a href="/lots" className="text-ink underline underline-offset-4">{d('allLots')}</a>
          </p>
        </div>
      </section>
      {state.status !== 'success' ? (
        <DataState
          kind={state.status}
          error={state.status === 'error' ? state.error : undefined}
          retry={retry}
        />
      ) : (
        <>
          <section className="flex flex-col gap-6">
            <h2 className="flex flex-wrap items-baseline gap-3 border-t border-ink pt-3.5">
              <span className="mono-label">01</span>
              {d('featured')}
              <Button asChild variant="link" className="ms-auto">
                <a href="/auctions">{d('allEvents')}</a>
              </Button>
            </h2>
            {selection!.featured.length ? (
              <LotGrid>
                {selection!.featured.map((event) => (
                  <EventCard key={event.id} event={event} now={now} />
                ))}
              </LotGrid>
            ) : (
              <DataState kind="empty" index="/auctions" />
            )}
          </section>
          <section className="flex flex-col gap-6">
            <h2 className="flex flex-wrap items-baseline gap-3 border-t border-ink pt-3.5">
              <span className="mono-label">02</span>
              {d('nearest')}
              <Button asChild variant="link" className="ms-auto">
                <a href="/lots">{d('allLots')}</a>
              </Button>
            </h2>
            {selection!.nearest.length ? (
              <LotGrid>
                {selection!.nearest.map((lot) => (
                  <PublicLotCard
                    key={lot.id}
                    lot={lot}
                    categories={state.response.data.categories}
                    now={now}
                  />
                ))}
              </LotGrid>
            ) : (
              <DataState
                kind={state.response.data.record.length ? 'noResults' : 'empty'}
              />
            )}
          </section>
          <RecommendationSection
            index="03"
            categories={state.response.data.categories}
          />
          <section className="flex flex-col gap-6">
            <h2 className="flex flex-wrap items-baseline gap-3 border-t border-ink pt-3.5">
              <span className="mono-label">04</span>
              {d('categories')}
            </h2>
            <div className="flex flex-wrap items-start gap-3">
              {state.response.data.categories.map((c) => (
                <Button asChild variant="outline" key={c.id}>
                  <a href={`/lots?category=${encodeURIComponent(c.slug)}`}>
                    {lang === 'vi' ? c.name_vi : c.name_en}
                  </a>
                </Button>
              ))}
            </div>
          </section>
        </>
      )}
    </div>
  )
}

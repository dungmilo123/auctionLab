import RecommendationSection from './RecommendationSection'
import * as m from 'motion/react-m'
import { ArrowUpRight } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { LotImage } from '@/components/site/Presentation'
import { useWebsiteReducedMotion, motionEase } from '@/components/site/motion'
import { displayEvent } from '@/lib/seed-display'
import { eventURL } from '@/lib/discovery-links'
import { useSiteCopy } from '@/lib/site-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { homeSelection } from '@/lib/discovery'
import { usePublicPage } from './usePublicPage'
import { loadHome } from './loads'
import { DataState, EventCard, PublicLotCard } from './DiscoveryPresentation'
export default function HomePage() {
  const { t } = useSiteCopy()
  const { lang, d } = useDiscoveryCopy()
  const { state, retry, now } = usePublicPage('home', loadHome)
  const selection =
    state.status === 'success'
      ? homeSelection(state.response.data.record, now)
      : null
  const reduce = useWebsiteReducedMotion()
  const leading = selection?.featured[0] ? displayEvent(selection.featured[0]) : null
  const entrance = state.status === 'success' && !reduce
  const enter = (delay: number) => ({ initial: { opacity: entrance ? 0 : 1, y: entrance ? 12 : 0 }, animate: { opacity: 1, y: 0 }, transition: { duration: entrance ? .5 : 0, delay: entrance ? delay : 0, ease: motionEase } })
  return (
    <div className="stack home-content">
      <section className="hero" key={state.status === 'success' ? 'published' : 'loading'}>
        <m.div className="stack" {...enter(0)}>
          <p className="hero-collection">{t('concept')}</p>
          <h1>{leading?.title ?? t('tagline')}</h1>
          <p className="lede">{leading?.description ?? t('intro')}</p>
          <m.div className="actions" {...enter(.15)}>
            <a className="site-button site-button-primary" href={leading ? eventURL(leading.slug) : '/lots'}>{leading ? d('parent') : d('allLots')}<Icon icon={ArrowUpRight} /></a>
            <a className="site-button site-button-link" href="/lots">{d('allLots')}</a>
          </m.div>
        </m.div>
        {leading && <m.figure className="hero-art" {...enter(.08)}><a href={eventURL(leading.slug)}><LotImage src={leading.cover_path} alt={leading.cover_alt} eager /></a><figcaption>{leading.theme}</figcaption></m.figure>}
        {state.status === 'loading' && <figure className="hero-art" aria-hidden="true"><div className="hero-placeholder" /><figcaption> </figcaption></figure>}
      </section>
      {state.status !== 'success' ? (
        <DataState
          kind={state.status}
          error={state.status === 'error' ? state.error : undefined}
          retry={retry}
        />
      ) : (
        <>
          <section className="stack home-section">
            <h2>{d('featured')}</h2>
            {selection!.featured.length ? (
              <div className="lot-grid">
                {selection!.featured.map((event) => (
                  <EventCard key={event.id} event={event} now={now} />
                ))}
              </div>
            ) : (
              <DataState kind="empty" index="/auctions" />
            )}
          </section>
          <section className="stack home-section">
            <h2>{d('nearest')}</h2>
            {selection!.nearest.length ? (
              <div className="lot-grid">
                {selection!.nearest.map((lot) => (
                  <PublicLotCard
                    key={lot.id}
                    lot={lot}
                    categories={state.response.data.categories}
                    now={now}
                  />
                ))}
              </div>
            ) : (
              <DataState
                kind={state.response.data.record.length ? 'noResults' : 'empty'}
              />
            )}
          </section>
          <RecommendationSection categories={state.response.data.categories} />
          <section className="stack home-section">
            <h2>{d('categories')}</h2>
            <div className="actions">
              {state.response.data.categories.map((c) => (
                <a
                  className="site-button site-button-outline"
                  key={c.id}
                  href={`/lots?category=${encodeURIComponent(c.slug)}`}
                >
                  {lang === 'vi' ? c.name_vi : c.name_en}
                </a>
              ))}
            </div>
          </section>
        </>
      )}
    </div>
  )
}

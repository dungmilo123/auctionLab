import cameraIllustration from '@/assets/illustrations/camera.svg'
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
  return (
    <div className="stack">
      <section className="hero">
        <div className="stack">
          <p className="eyebrow">AuctionLab · {t('concept')}</p>
          <h1>{t('tagline')}</h1>
          <p className="lede">{t('intro')}</p>
          <div className="actions">
            <a className="site-button site-button-primary" href="/lots">
              {d('allLots')}
            </a>
            <a className="site-button site-button-outline" href="/auctions">
              {d('allEvents')}
            </a>
          </div>
        </div>
        <figure className="hero-art">
          <img src={cameraIllustration} alt="" width="640" height="480" />
          <figcaption>{t('illustration')}</figcaption>
        </figure>
      </section>
      {state.status !== 'success' ? (
        <DataState
          kind={state.status}
          error={state.status === 'error' ? state.error : undefined}
          retry={retry}
        />
      ) : (
        <>
          <section className="stack">
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
          <section className="stack">
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
          <section className="stack">
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

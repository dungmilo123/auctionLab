import RecommendationSection from './RecommendationSection'
import {recordLotObservation} from '@/lib/lot-observation'
import {useEffect} from 'react'
import WatchControl from '@/routes/account/WatchControl'
import { lotURL } from '@/lib/discovery-links'
import { useCallback } from 'react'
import { publicCatalog } from '@/api/public-catalog'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import AuctionPanel from './AuctionPanel'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { usePublicPage } from './usePublicPage'
import { withCategories } from './loads'
import { eventURL } from '@/lib/discovery-links'
import {
  CategoryName,
  DataState,
  Demo,
  PlannedBadge,
  Times,
  Gallery,
} from './DiscoveryPresentation'
export default function LotDetailPage({ slug }: { slug: string }) {
  const { d } = useDiscoveryCopy()
  const load = useCallback(
    (signal: AbortSignal) =>
      withCategories(publicCatalog.lot(slug, signal), signal),
    [slug],
  )
  const { state, retry, now } = usePublicPage(slug, load)
  const observedID=state.status==='success'?state.response.data.record.id:null
  useEffect(()=>{if(observedID)recordLotObservation(observedID)},[observedID])
  if (state.status !== 'success')
    return (
      <DataState
        kind={state.status}
        error={state.status === 'error' ? state.error : undefined}
        retry={retry}
      />
    )
  const { record: lot, categories } = state.response.data
  const f = lot.fulfillment
  return (
    <article className="stack">
      <Demo demo={lot.demo} />
      <h1>{lot.title}</h1>
      <p className="eyebrow">
        <CategoryName id={lot.category_id} categories={categories} />
      </p>
      <div className="detail-grid">
        <Gallery key={lot.id} images={lot.images} />
        <section className="stack">
          <h2>{d('description')}</h2>
          <p className="editorial-text">{lot.description}</p>
          <dl className="metadata">
            {lot.brand && (
              <div>
                <dt>{d('brand')}</dt>
                <dd>{lot.brand}</dd>
              </div>
            )}
            {lot.era && (
              <div>
                <dt>{d('era')}</dt>
                <dd>{lot.era}</dd>
              </div>
            )}
          </dl>
          <PlannedBadge open={lot.opens_at} close={lot.closes_at} now={now} />
          <Times open={lot.opens_at} close={lot.closes_at} />
          <a href={eventURL(lot.event_slug)}>{d('parent')}</a>
        </section>
      </div>
      <section className="stack-small">
        <h2>{d('condition')}</h2>
        <p className="editorial-text">{lot.condition}</p>
      </section>
      {lot.specifications.length > 0 && (
        <section className="stack-small">
          <h2>{d('specs')}</h2>
          <dl className="specifications">
            {lot.specifications.map((s, i) => (
              <div key={i}>
                <dt>{s.name}</dt>
                <dd>{s.value}</dd>
              </div>
            ))}
          </dl>
        </section>
      )}
      {lot.story && (
        <section className="stack-small">
          <h2>{d('story')}</h2>
          <p className="editorial-text">{lot.story}</p>
        </section>
      )}
      <section className="empty-state stack">
        <h2>{d('fulfillment')}</h2>
        {f.shipping_enabled ? (
          <div className="stack-small">
            <p className="numeric">
              {d('shipping')}: {formatCatalogMoney(f.shipping_fee_minor)}
            </p>
            <p className="editorial-text">{f.shipping_instructions}</p>
          </div>
        ) : (
          <p>{d('noShipping')}</p>
        )}
        {f.pickup_enabled ? (
          <div className="stack-small">
            <h3>{d('pickup')}</h3>
            <p className="editorial-text">{f.pickup_instructions}</p>
          </div>
        ) : (
          <p>{d('noPickup')}</p>
        )}
      </section>
      <WatchControl id={lot.id} target={lotURL(lot.slug)} />
      <AuctionPanel key={lot.id} id={lot.id} target={lotURL(lot.slug)} />
      <RecommendationSection source={lot.slug} categories={categories}/>
    </article>
  )
}

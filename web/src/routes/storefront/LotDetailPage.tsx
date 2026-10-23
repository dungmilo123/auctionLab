import RecommendationSection from './RecommendationSection'
import { recordLotObservation } from '@/lib/lot-observation'
import { useEffect, useCallback } from 'react'
import WatchControl from '@/routes/account/WatchControl'
import { lotURL, eventURL } from '@/lib/discovery-links'
import { publicCatalog } from '@/api/public-catalog'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import AuctionPanel from './AuctionPanel'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { displayLot } from '@/lib/seed-display'
import { usePublicPage } from './usePublicPage'
import { withCategories } from './loads'
import { CategoryName, DataState, Gallery } from './DiscoveryPresentation'
export default function LotDetailPage({ slug }: { slug: string }) {
  const { d } = useDiscoveryCopy()
  const load = useCallback((signal: AbortSignal) => withCategories(publicCatalog.lot(slug, signal), signal), [slug])
  const { state, retry } = usePublicPage(slug, load)
  const observedID = state.status === 'success' ? state.response.data.record.id : null
  useEffect(() => { if (observedID) recordLotObservation(observedID) }, [observedID])
  if (state.status !== 'success') return <DataState kind={state.status} error={state.status === 'error' ? state.error : undefined} retry={retry} />
  const { record, categories } = state.response.data, lot = displayLot(record), f = lot.fulfillment
  const terms = <div className="stack lot-terms">
    <section className="stack-small"><h2>{d('condition')}</h2><p className="editorial-text">{lot.condition}</p></section>
    <section className="stack-small"><h2>{d('fulfillment')}</h2>
      {f.shipping_enabled ? <div className="stack-small"><p className="numeric">{d('shipping')}: {formatCatalogMoney(f.shipping_fee_minor)}</p><p className="editorial-text">{f.shipping_instructions}</p></div> : <p>{d('noShipping')}</p>}
      {f.pickup_enabled ? <div className="stack-small"><h3>{d('pickup')}</h3><p className="editorial-text">{f.pickup_instructions}</p></div> : <p>{d('noPickup')}</p>}
    </section><WatchControl lot={record} target={lotURL(lot.slug)} />
  </div>
  return <article className="stack">
    <p className="muted"><a href={eventURL(lot.event_slug)}>{d('parent')}</a> / <CategoryName id={lot.category_id} categories={categories} /></p>
    <h1>{lot.title}</h1>
    <div className="detail-grid"><Gallery key={lot.id} images={lot.images} /><div className="auction-column"><AuctionPanel key={lot.id} id={lot.id} target={lotURL(lot.slug)} beforeBid={terms} /></div></div>
    <section className="stack lot-story"><h2>{d('description')}</h2><p className="editorial-text">{lot.description}</p>
      {(lot.brand || lot.era) && <dl className="metadata">{lot.brand && <div><dt>{d('brand')}</dt><dd>{lot.brand}</dd></div>}{lot.era && <div><dt>{d('era')}</dt><dd>{lot.era}</dd></div>}</dl>}
      {lot.story && <><h2>{d('story')}</h2><p className="editorial-text">{lot.story}</p></>}
      {lot.specifications.length > 0 && <><h2>{d('specs')}</h2><dl className="specifications">{lot.specifications.map((s, i) => <div key={i}><dt>{s.name}</dt><dd>{s.value}</dd></div>)}</dl></>}
    </section>
    <RecommendationSection source={lot.slug} categories={categories} />
  </article>
}

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
function Row({ title, open, children }: { title: string; open?: boolean; children: React.ReactNode }) { return <details open={open} className="group border-t border-dashed border-ink/25"><summary className="mono-label flex min-h-11 cursor-pointer items-center justify-between">{title}<span aria-hidden="true" className="group-open:rotate-45 transition-transform">+</span></summary><div className="flex flex-col gap-3 pb-4">{children}</div></details> }
export default function LotDetailPage({ slug }: { slug: string }) {
  const { d } = useDiscoveryCopy()
  const load = useCallback((signal: AbortSignal) => withCategories(publicCatalog.lot(slug, signal), signal), [slug])
  const { state, retry } = usePublicPage(slug, load)
  const observedID = state.status === 'success' ? state.response.data.record.id : null
  useEffect(() => { if (observedID) recordLotObservation(observedID) }, [observedID])
  if (state.status !== 'success') return <DataState kind={state.status} error={state.status === 'error' ? state.error : undefined} retry={retry} />
  const { record, categories } = state.response.data, lot = displayLot(record), f = lot.fulfillment
  return <article className="flex flex-col gap-8">
    <p className="mono-label"><a href={eventURL(lot.event_slug)}>{d('parent')}</a> / <CategoryName id={lot.category_id} categories={categories} /></p>
    <div className="grid items-start gap-8 lg:grid-cols-[1.3fr_1fr]">
      <Gallery key={lot.id} images={lot.images} />
      <div className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper p-6 max-sm:p-4">
        <header className="flex flex-col gap-2"><h1>{lot.title}</h1>
          {(lot.brand || lot.era) && <p className="font-mono text-sm text-muted-foreground">{[lot.brand, lot.era].filter(Boolean).join(' · ')}</p>}</header>
        <AuctionPanel key={lot.id} id={lot.id} target={lotURL(lot.slug)} beforeBid={<WatchControl lot={record} target={lotURL(lot.slug)} />} />
      </div>
    </div>
    <div className="flex max-w-[70ch] flex-col border-b border-dashed border-ink/25">
      <Row title={d('description')} open><p className="whitespace-pre-wrap">{lot.description}</p></Row>
      {lot.specifications.length > 0 && <Row title={d('specs')}><dl className="flex flex-col">{lot.specifications.map((s, i) => <div key={i} className="flex gap-3 border-b border-dashed border-ink/15 py-1.5"><dt>{s.name}</dt><dd className="m-0 ms-auto text-end">{s.value}</dd></div>)}</dl></Row>}
      <Row title={d('condition')}><p className="whitespace-pre-wrap">{lot.condition}</p></Row>
      <Row title={d('fulfillment')}>
        {f.shipping_enabled ? <><p className="font-mono text-sm numeric">{d('shipping')}: {formatCatalogMoney(f.shipping_fee_minor)}</p><p className="whitespace-pre-wrap">{f.shipping_instructions}</p></> : <p>{d('noShipping')}</p>}
        {f.pickup_enabled ? <><h3>{d('pickup')}</h3><p className="whitespace-pre-wrap">{f.pickup_instructions}</p></> : <p>{d('noPickup')}</p>}
      </Row>
      {lot.story && <Row title={d('story')}><p className="whitespace-pre-wrap">{lot.story}</p></Row>}
    </div>
    <RecommendationSection source={lot.slug} categories={categories} />
  </article>
}

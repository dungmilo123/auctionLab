import { displayEvent } from '@/lib/seed-display'
import { useCallback } from 'react'
import { publicCatalog } from '@/api/public-catalog'
import { LotImage } from '@/components/site/Presentation'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { usePublicPage } from './usePublicPage'
import { withCategories } from './loads'
import {
  DataState,
  PlannedBadge,
  Times,
  PublicLotCard,
} from './DiscoveryPresentation'
export default function AuctionDetailPage({ slug }: { slug: string }) {
  const { d } = useDiscoveryCopy()
  const load = useCallback(
    (signal: AbortSignal) =>
      withCategories(publicCatalog.event(slug, signal), signal),
    [slug],
  )
  const { state, retry, now } = usePublicPage(slug, load)
  if (state.status !== 'success')
    return (
      <DataState
        kind={state.status}
        error={state.status === 'error' ? state.error : undefined}
        retry={retry}
        index="/auctions"
      />
    )
  const { record, categories } = state.response.data
  const event = displayEvent(record)
  return (
    <article className="stack">
      <h1>{event.title}</h1>
      <p className="lede">{event.theme}</p>
      <div className="detail-grid">
        <LotImage src={event.cover_path} alt={event.cover_alt} eager />
        <div className="stack">
          <p className="editorial-text">{event.description}</p>
          <PlannedBadge
            open={event.opens_at}
            close={event.planned_end}
            now={now}
          />
          <Times open={event.opens_at} close={event.planned_end} event />
        </div>
      </div>
      <h2>{d('lots')}</h2>
      {event.lots.length ? (
        <div className="lot-grid">
          {[...event.lots]
            .sort((a, b) => a.position - b.position)
            .map((lot) => (
              <PublicLotCard
                key={lot.id}
                lot={lot}
                categories={categories}
                now={now}
              />
            ))}
        </div>
      ) : (
        <DataState kind="empty" />
      )}
    </article>
  )
}

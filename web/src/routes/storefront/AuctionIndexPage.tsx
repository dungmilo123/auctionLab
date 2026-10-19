import { useSiteCopy } from '@/lib/site-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { orderEvents, phase, phases, query } from '@/lib/discovery'
import { usePublicPage } from './usePublicPage'
import { loadEvents } from './loads'
import { DataState, EventCard } from './DiscoveryPresentation'
export function PhaseControl({ value }: { value: string }) {
  const { d } = useDiscoveryCopy()
  return (
    <label className="stack-small">
      {d('phase')}
      <select className="site-input" name="phase" defaultValue={value}>
        {phases.map((p) => (
          <option key={p} value={p}>
            {d(p)}
          </option>
        ))}
      </select>
    </label>
  )
}
export default function AuctionIndexPage() {
  const { t } = useSiteCopy()
  const { d } = useDiscoveryCopy()
  const { state, retry, now } = usePublicPage('events', loadEvents)
  const selection = query(window.location.search, [])
  const events =
    state.status === 'success'
      ? orderEvents(state.response.data, now).filter(
          (e) =>
            selection.phase === 'all' ||
            phase(e.opens_at, e.planned_end, now) === selection.phase,
        )
      : []
  return (
    <div className="stack">
      <h1>{t('auctions')}</h1>
      <form action="/auctions" method="get" className="discovery-controls">
        <PhaseControl value={selection.phase} />
        <button className="site-button site-button-primary" type="submit">
          {d('apply')}
        </button>
        <a href="/auctions">{d('clear')}</a>
      </form>
      {state.status !== 'success' ? (
        <DataState
          kind={state.status}
          error={state.status === 'error' ? state.error : undefined}
          retry={retry}
          index="/auctions"
        />
      ) : (
        <>
          <p>
            {d('count')}: {events.length}
          </p>
          {events.length ? (
            <div className="lot-grid">
              {events.map((e) => (
                <EventCard key={e.id} event={e} now={now} />
              ))}
            </div>
          ) : (
            <DataState
              kind={state.response.data.length ? 'noResults' : 'empty'}
              index="/auctions"
            />
          )}
        </>
      )}
    </div>
  )
}

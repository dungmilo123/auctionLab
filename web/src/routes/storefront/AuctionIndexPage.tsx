import { useSiteCopy } from '@/lib/site-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { orderEvents, phase, phases, query } from '@/lib/discovery'
import { usePublicPage } from './usePublicPage'
import { loadEvents } from './loads'
import { DataState, EventCard, LotGrid } from './DiscoveryPresentation'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
export function PhaseControl({ value, id = 'phase-filter' }: { value: string; id?: string }) {
  const { d } = useDiscoveryCopy()
  return (
    <div className="flex min-w-0 flex-col gap-2.5">
      <Label htmlFor={id}>{d('phase')}</Label>
      <Select id={id} name="phase" defaultValue={value}>
        {phases.map((p) => (
          <option key={p} value={p}>
            {d(p)}
          </option>
        ))}
      </Select>
    </div>
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
    <div className="flex flex-col gap-6">
      <h1>{t('auctions')}</h1>
      <form action="/auctions" method="get" className="grid gap-4 border-y border-hairline py-6 sm:grid-cols-2 lg:grid-cols-4">
        <PhaseControl value={selection.phase} />
        <div className="flex min-w-0 flex-wrap items-end gap-3">
          <Button type="submit">{d('apply')}</Button>
          <Button asChild variant="link"><a href="/auctions">{d('clear')}</a></Button>
        </div>
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
          <p className="mono-label">
            {d('count')}: {events.length}
          </p>
          {events.length ? (
            <LotGrid>
              {events.map((e) => (
                <EventCard key={e.id} event={e} now={now} />
              ))}
            </LotGrid>
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

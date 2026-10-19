import { useSiteCopy } from '@/lib/site-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { query, selectLots, sorts } from '@/lib/discovery'
import { usePublicPage } from './usePublicPage'
import { loadLots } from './loads'
import { DataState, PublicLotCard } from './DiscoveryPresentation'
import { PhaseControl } from './AuctionIndexPage'
export default function LotIndexPage() {
  const { t } = useSiteCopy()
  const { lang, d } = useDiscoveryCopy()
  const { state, retry, now } = usePublicPage('lots', loadLots)
  if (state.status !== 'success')
    return (
      <div className="stack">
        <h1>{t('lots')}</h1>
        <DataState
          kind={state.status}
          error={state.status === 'error' ? state.error : undefined}
          retry={retry}
        />
      </div>
    )
  const { record: lots, categories } = state.response.data
  const selection = query(window.location.search, categories)
  const matches = selectLots(lots, selection, categories, now)
  return (
    <div className="stack">
      <h1>{t('lots')}</h1>
      <form action="/lots" method="get" className="discovery-controls">
        <label className="stack-small">
          {d('q')}
          <input
            className="site-input"
            name="q"
            type="search"
            defaultValue={selection.q}
          />
        </label>
        <label className="stack-small">
          {d('category')}
          <select
            className="site-input"
            name="category"
            defaultValue={selection.category}
          >
            <option value="">{d('allCategories')}</option>
            {categories.map((c) => (
              <option key={c.id} value={c.slug}>
                {lang === 'vi' ? c.name_vi : c.name_en}
              </option>
            ))}
          </select>
        </label>
        <PhaseControl value={selection.phase} />
        <label className="stack-small">
          {d('sort')}
          <select
            className="site-input"
            name="sort"
            defaultValue={selection.sort}
          >
            {sorts.map((s) => (
              <option key={s} value={s}>
                {d(s)}
              </option>
            ))}
          </select>
        </label>
        <button className="site-button site-button-primary" type="submit">
          {d('apply')}
        </button>
        <a href="/lots">{d('clear')}</a>
      </form>
      <p role="status">
        {d('count')}: {matches.length}
      </p>
      {matches.length ? (
        <div className="lot-grid">
          {matches.map((lot) => (
            <PublicLotCard
              key={lot.id}
              lot={lot}
              categories={categories}
              now={now}
            />
          ))}
        </div>
      ) : (
        <DataState kind={lots.length ? 'noResults' : 'empty'} />
      )}
    </div>
  )
}

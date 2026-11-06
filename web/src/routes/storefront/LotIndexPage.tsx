import Disclosure from '@/components/site/Disclosure'
import { useSiteCopy } from '@/lib/site-copy'
import { useDiscoveryCopy } from '@/lib/discovery-copy'
import { query, selectLots, sorts } from '@/lib/discovery'
import { usePublicPage } from './usePublicPage'
import { loadLots } from './loads'
import { DataState, LotGrid, PublicLotCard } from './DiscoveryPresentation'
import { PhaseControl } from './AuctionIndexPage'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
export default function LotIndexPage() {
  const { t } = useSiteCopy()
  const { lang, d } = useDiscoveryCopy()
  const { state, retry, now } = usePublicPage('lots', loadLots)
  if (state.status !== 'success')
    return (
      <div className="flex flex-col gap-6">
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
    <div className="flex flex-col gap-6">
      <h1>{t('lots')}</h1>
      <form action="/lots" method="get" className="grid gap-4 border-y border-hairline py-6 sm:grid-cols-2 lg:grid-cols-4">
        <div className="flex min-w-0 flex-col gap-2.5">
          <Label htmlFor="lot-q">{d('q')}</Label>
          <Input id="lot-q" name="q" type="search" defaultValue={selection.q} />
        </div>
        <Disclosure label={d('filters')}>
          <div className="flex min-w-0 flex-col gap-2.5">
            <Label htmlFor="lot-category">{d('category')}</Label>
            <Select id="lot-category" name="category" defaultValue={selection.category}>
              <option value="">{d('allCategories')}</option>
              {categories.map((c) => (
                <option key={c.id} value={c.slug}>
                  {lang === 'vi' ? c.name_vi : c.name_en}
                </option>
              ))}
            </Select>
          </div>
          <PhaseControl value={selection.phase} id="lot-phase" />
          <div className="flex min-w-0 flex-col gap-2.5">
            <Label htmlFor="lot-sort">{d('sort')}</Label>
            <Select id="lot-sort" name="sort" defaultValue={selection.sort}>
              {sorts.map((s) => (
                <option key={s} value={s}>
                  {d(s)}
                </option>
              ))}
            </Select>
          </div>
        </Disclosure>
        <div className="flex min-w-0 flex-wrap items-end gap-3">
          <Button type="submit">{d('apply')}</Button>
          <Button asChild variant="link"><a href="/lots">{d('clear')}</a></Button>
        </div>
      </form>
      <p role="status" className="mono-label">
        {d('count')}: {matches.length}
      </p>
      {matches.length ? (
        <LotGrid>
          {matches.map((lot) => (
            <PublicLotCard
              key={lot.id}
              lot={lot}
              categories={categories}
              now={now}
            />
          ))}
        </LotGrid>
      ) : (
        <DataState kind={lots.length ? 'noResults' : 'empty'} />
      )}
    </div>
  )
}

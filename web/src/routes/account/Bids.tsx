import { useOrderCopy } from '@/lib/order-copy'
import { useEffect, useState } from 'react'
import { auctionAPI } from '@/api/auction'
import { publicCatalog } from '@/api/public-catalog'
import { useParticipation } from '@/auction/useParticipation'
import { filteredParticipation, participationFilters, type ParticipationFilter } from '@/auction/eligibility'
import { useAuctionCopy } from '@/lib/auction-copy'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { lotURL } from '@/lib/discovery-links'
import { standingStyle } from '@/lib/standing'
import { Standing } from '@/components/site/Standing'
import { LotImage } from '@/components/site/Presentation'
import { Button } from '@/components/ui/button'

type LotFacts = { image?: string; alt: string; closesAt: number | null }

/**
 * Pictures and planned closes come from the public catalogue (one read). The
 * clock is anchored to that read's server time so countdowns match the lot page.
 * A failed read only hides pictures and countdowns; bids still render.
 */
function useLotFacts() {
  const [facts, setFacts] = useState<Map<string, LotFacts>>(new Map())
  const [offset, setOffset] = useState(0)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const controller = new AbortController()
    publicCatalog.lots(controller.signal).then(r => {
      setOffset(r.serverTime - (Date.now() - (performance.now() - r.receivedAt)))
      setFacts(new Map(r.data.map(l => [l.slug, { image: l.images[0]?.path, alt: l.images[0]?.alt ?? '', closesAt: l.closes_at }])))
    }, () => {})
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => { controller.abort(); clearInterval(timer) }
  }, [])
  return { facts, now: now + offset }
}

export default function Bids(){
  const {b,lang}=useAuctionCopy();const {o}=useOrderCopy();const {state,retry}=useParticipation(auctionAPI.list);const [filter,setFilter]=useState<ParticipationFilter>('all')
  const {facts,now}=useLotFacts()
  if(state.status==='loading')return <p role="status">{b('loading')}</p>
  if(state.status==='error')return <section className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8"><p role="alert">{b('listUnavailable')}</p><Button variant="outline" className="self-start" onClick={retry}>{b('retry')}</Button></section>
  const rows=filteredParticipation(state.data,filter)
  return <section className="flex flex-col gap-6"><nav className="flex flex-wrap items-start gap-3" aria-label={b('myBids')}>{participationFilters.map(f=><Button variant={f===filter?'default':'outline'} key={f} onClick={()=>setFilter(f)} aria-pressed={f===filter}>{b(f)}</Button>)}</nav>
    {rows.length===0?<p className="border border-hairline border-t-ink bg-paper px-6 py-8 text-muted-foreground">{b('empty')}</p>:<ul className="grid gap-4">{rows.map(p=>{
      const lot=facts.get(p.slug);const style=standingStyle[p.status]
      return <li data-standing-row={p.status} className={`grid grid-cols-[minmax(0,9rem)_minmax(0,1fr)] items-start gap-5 border border-hairline border-l-4 bg-paper p-5 max-sm:grid-cols-1 ${style?.edge??''}`} key={p.lot_id}>
        <a href={lotURL(p.slug)} tabIndex={-1} aria-hidden="true" className="block border border-hairline bg-secondary"><LotImage src={lot?.image} alt={lot?.alt??''} plain /></a>
        <div className="flex min-w-0 flex-col gap-3">
          <h2><a href={lotURL(p.slug)}>{p.title}</a></h2>
          <Standing status={p.status} closesAt={lot?.closesAt??null} now={now} compact />
          <dl className="grid grid-cols-2 gap-3 numeric max-sm:grid-cols-1">
            <div className={`border-l-4 bg-secondary px-3.5 py-2.5 ${style?.edge??'border-l-ink/30'}`}><dt className="mono-label">{b('personal')}</dt><dd className={`mt-1 font-mono text-xl font-semibold ${style?.text??'text-ink'}`}>{formatCatalogMoney(p.latest?.amount_minor??null, lang)}</dd></div>
            <div className="border-l-4 border-l-[var(--standing-leading)] bg-[var(--standing-leading-bg)] px-3.5 py-2.5"><dt className="mono-label">{b('current')}</dt><dd className="mt-1 font-mono text-xl font-semibold text-[var(--standing-leading)]">{formatCatalogMoney(p.current_price_minor, lang)}</dd></div>
          </dl>
          {p.status==='won'&&p.order_id&&<Button asChild className="self-start"><a href={`/account/checkout/${p.order_id}`}>{o(p.checkout_available?'open':'viewPaid')}</a></Button>}
        </div>
      </li>})}</ul>}
  </section>
}

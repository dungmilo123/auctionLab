import { useOrderCopy } from '@/lib/order-copy'
import { useState } from 'react'
import { auctionAPI } from '@/api/auction'
import { useParticipation } from '@/auction/useParticipation'
import { filteredParticipation, participationFilters, type ParticipationFilter } from '@/auction/eligibility'
import { useAuctionCopy } from '@/lib/auction-copy'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { lotURL } from '@/lib/discovery-links'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
export default function Bids(){
  const {b}=useAuctionCopy();const {o}=useOrderCopy();const {state,retry}=useParticipation(auctionAPI.list);const [filter,setFilter]=useState<ParticipationFilter>('all')
  if(state.status==='loading')return <p role="status">{b('loading')}</p>
  if(state.status==='error')return <section className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8"><p role="alert">{b('listUnavailable')}</p><Button variant="outline" className="self-start" onClick={retry}>{b('retry')}</Button></section>
  const rows=filteredParticipation(state.data,filter)
  return <section className="flex flex-col gap-6"><nav className="flex flex-wrap items-start gap-3" aria-label={b('myBids')}>{participationFilters.map(f=><Button variant={f===filter?'default':'outline'} key={f} onClick={()=>setFilter(f)} aria-pressed={f===filter}>{b(f)}</Button>)}</nav>
    {rows.length===0?<p className="border border-hairline border-t-ink bg-paper px-6 py-8 text-muted-foreground">{b('empty')}</p>:<ul className="grid gap-4">{rows.map(p=><li className="flex flex-col gap-2.5 border border-hairline bg-paper p-5" key={p.lot_id}><h2><a href={lotURL(p.slug)}>{p.title}</a></h2><Badge tone={p.status==='won'?'live':'neutral'}>{b(p.status)}</Badge><dl className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,9rem),1fr))] gap-4 border-y border-hairline py-4 numeric"><div><dt className="mono-label">{b('personal')}</dt><dd className="mt-2">{formatCatalogMoney(p.latest?.amount_minor??null)}</dd></div><div><dt className="mono-label">{b('current')}</dt><dd className="mt-2">{formatCatalogMoney(p.current_price_minor)}</dd></div></dl>{p.status==='won'&&p.order_id&&<Button asChild className="self-start"><a href={`/account/checkout/${p.order_id}`}>{o(p.checkout_available?'open':'viewPaid')}</a></Button>}</li>)}</ul>}
  </section>
}

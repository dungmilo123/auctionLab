import { useOrderCopy } from '@/lib/order-copy'
import { useState } from 'react'
import { auctionAPI } from '@/api/auction'
import { useParticipation } from '@/auction/useParticipation'
import { filteredParticipation, participationFilters, type ParticipationFilter } from '@/auction/eligibility'
import { useAuctionCopy } from '@/lib/auction-copy'
import { formatCatalogMoney } from '@/lib/catalog-money'
import { lotURL } from '@/lib/discovery-links'
export default function Bids(){
  const {b}=useAuctionCopy();const {o}=useOrderCopy();const {state,retry}=useParticipation(auctionAPI.list);const [filter,setFilter]=useState<ParticipationFilter>('all')
  if(state.status==='loading')return <p role="status">{b('loading')}</p>
  if(state.status==='error')return <section className="empty-state stack"><p role="alert">{b('listUnavailable')}</p><button className="site-button" onClick={retry}>{b('retry')}</button></section>
  const rows=filteredParticipation(state.data,filter)
  return <section className="stack"><nav className="actions auction-filters" aria-label={b('myBids')}>{participationFilters.map(f=><button className="site-button site-button-outline" key={f} onClick={()=>setFilter(f)} aria-pressed={f===filter}>{b(f)}</button>)}</nav>
    {rows.length===0?<p className="empty-state">{b('empty')}</p>:<ul className="auction-participation">{rows.map(p=><li className="catalog-record stack-small" key={p.lot_id}><h2><a href={lotURL(p.slug)}>{p.title}</a></h2><p className="status-badge">{b(p.status)}</p><dl className="metadata numeric"><div><dt>{b('personal')}</dt><dd>{formatCatalogMoney(p.latest?.amount_minor??null)}</dd></div><div><dt>{b('current')}</dt><dd>{formatCatalogMoney(p.current_price_minor)}</dd></div></dl>{p.status==='won'&&p.order_id&&<a className="site-button" href={`/account/checkout/${p.order_id}`}>{o(p.checkout_available?'open':'viewPaid')}</a>}</li>)}</ul>}
  </section>
}

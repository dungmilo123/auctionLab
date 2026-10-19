import type { AuctionResponse, Participation } from '@/api/auction'
export function mayBid(response:AuctionResponse|null,now:number,stale:boolean,participation:Participation|null,pending:boolean){
 const s=response?.data
 return !!s&&!stale&&!pending&&s.auction_state==='open'&&now>=s.opens_at&&now<s.closes_at&&!s.amount_limit_reached&&s.next_minimum_minor!==null&&!!participation&&participation.lot_id===s.lot_id&&participation.revision===s.revision&&participation.status!=='leading'&&participation.status!=='closing'&&participation.status!=='won'&&participation.status!=='lost'
}
export const participationFilters=['all','leading','outbid','won','lost'] as const
export type ParticipationFilter=typeof participationFilters[number]
export function filteredParticipation<T extends {status:string}>(rows:T[],filter:ParticipationFilter){return filter==='all'?rows:rows.filter(row=>row.status===filter)}

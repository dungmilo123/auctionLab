import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode, type FormEvent } from 'react'
import { auctionAPI, AuctionError, type AuctionBid, type AuctionResponse, type Intent } from '@/api/auction'
import { AuctionFeed } from '@/auction/feed'
import { mayBid } from '@/auction/eligibility'
import { useParticipation } from '@/auction/useParticipation'
import { useAuctionCopy, auctionErrorKey, type AuctionKey } from '@/lib/auction-copy'
import { formatCatalogMoney, parseCatalogMoney, parseDongMoney } from '@/lib/catalog-money'
import { useOrderCopy } from '@/lib/order-copy'
import { useAccount } from '@/account/AccountProvider'
import { AccountStatus } from '@/routes/account/AccountStatus'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Standing } from '@/components/site/Standing'

function BidForm({ id, response, now, stale, accept, refresh }: { id:string;response:AuctionResponse|null;now:number;stale:boolean;accept:(s:AuctionResponse)=>void;refresh:()=>Promise<void> }) {
  const { b, lang }=useAuctionCopy();const {o}=useOrderCopy();const {store}=useAccount();const [value,setValue]=useState('');const [intent,setIntent]=useState<Intent|null>(null);const [sending,setSending]=useState(false);const [message,setMessage]=useState<AuctionKey|null>(null)
  const pending=useRef<Intent|null>(null);const alive=useRef(true);const operation=useRef<AbortController|null>(null)
  useEffect(()=>{alive.current=true;const hide=()=>{alive.current=false;pending.current=null;operation.current?.abort();setIntent(null);setMessage(null);setSending(false)};window.addEventListener('pagehide',hide);return()=>{alive.current=false;pending.current=null;operation.current?.abort();window.removeEventListener('pagehide',hide)}},[])
  const load=useCallback((signal:AbortSignal)=>auctionAPI.participation(id,signal),[id]);const {state,retry}=useParticipation(load,response?.data.revision)
  useEffect(()=>{if(state.status==='success'&&pending.current&&state.data.bids.some(v=>v.request_id===pending.current?.request_id)){operation.current?.abort();pending.current=null;setIntent(null);setSending(false);setMessage('accepted')}},[state])
  const participation=state.status==='success'?state.data:null
  const eligible=mayBid(response,now,stale,participation,intent!==null)
  async function send(next:Intent){
    if(sending)return;setSending(true);setMessage(null);operation.current?.abort();const controller=new AbortController();operation.current=controller
    try{
      const result=await auctionAPI.submit(next,controller.signal)
      if(!alive.current||controller.signal.aborted)return
      if(result.snapshot)accept(result.snapshot)
      pending.current=null;setIntent(null);setMessage('accepted');setValue('');retry();void refresh()
    }catch(e){
      if(!alive.current||controller.signal.aborted)return
      const error=e instanceof AuctionError?e:new AuctionError('unavailable')
      if(error.snapshot)accept(error.snapshot)
      if(error.status===401){pending.current=null;store.signedOut();return}
      if(pending.current?.request_id!==next.request_id)return
      if(error.status>=400&&error.status<500){pending.current=null;setIntent(null);setMessage(auctionErrorKey(error.code))}
      else setMessage('unknown')
      retry();void refresh()
    }finally{if(alive.current&&!controller.signal.aborted)setSending(false)}
  }
  function submit(e:FormEvent){e.preventDefault();if(!eligible||pending.current)return
    let amount:number|null;try{amount=(lang==='vi'?parseDongMoney:parseCatalogMoney)(value)}catch{setMessage('invalid_amount');return};if(amount===null){setMessage('invalid_amount');return}
    const next={lot_id:id,amount_minor:amount,request_id:crypto.randomUUID()};pending.current=next;setIntent(next);void send(next)
  }
  return <div className="flex flex-col gap-6">
    {state.status==='success'?<Standing status={state.data.status} closesAt={response?.data.closes_at??null} now={now}/>:<p>{b(state.status==='loading'?'identity':'privateUnavailable')}</p>}
    {participation?.order_id&&<Button asChild><a href={`/account/checkout/${participation.order_id}`}>{o(participation.checkout_available?'open':'viewPaid')}</a></Button>}
    {state.status==='error'&&<Button variant="outline" type="button" onClick={retry}>{b('retry')}</Button>}
    <form className="flex flex-col gap-2.5" onSubmit={submit}>
      <Label htmlFor={`bid-${id}`}>{b('amount')}</Label>
      <Input id={`bid-${id}`} inputMode="decimal" autoComplete="off" value={value} readOnly={!eligible} aria-disabled={!eligible} onChange={e=>setValue(e.target.value)} aria-describedby={`bid-help-${id} bid-message-${id}`} />
      <p id={`bid-help-${id}`} className="text-sm text-muted-foreground">{b('help')}</p>
      <Button type="submit" className="self-start" aria-disabled={!eligible}>{b(sending?'sending':'submit')}</Button>
    </form>
    <p className={message==='accepted'?'motion-safe:animate-[confirmed-feedback_150ms_ease-out]':undefined} id={`bid-message-${id}`} role="status" aria-live="polite">{message?b(message):''}</p>
    {intent&&!sending&&<div className="flex flex-col gap-2.5"><p>{b('unknown')}</p><div className="flex flex-wrap items-start gap-3"><Button variant="outline" type="button" onClick={retry}>{b('reconcile')}</Button><Button variant="outline" type="button" onClick={()=>void send(intent)}>{b('sameRetry')}</Button></div></div>}
  </div>
}
export default function AuctionPanel({ id,target, beforeBid }:{id:string;target:string;beforeBid?:ReactNode}){
  const {b,lang}=useAuctionCopy();const {state:account}=useAccount();const feed=useMemo(()=>new AuctionFeed(id),[id]);const state=useSyncExternalStore(feed.subscribe,feed.get,feed.get)
  const [older,setOlder]=useState<AuctionBid[]>([]);const [historyPending,setHistoryPending]=useState(false);const [historyError,setHistoryError]=useState(false);const [hasOlder,setHasOlder]=useState<boolean|null>(null)
  const alive=useRef(true);const historyController=useRef<AbortController|null>(null)
  useEffect(()=>{alive.current=true;feed.start();const resume=()=>{if(!document.hidden){feed.stop();feed.start()}};const visibility=()=>{if(document.hidden)feed.stop();else resume()};window.addEventListener('focus',resume);window.addEventListener('pagehide',feed.stop);document.addEventListener('visibilitychange',visibility);return()=>{alive.current=false;historyController.current?.abort();feed.stop();window.removeEventListener('focus',resume);window.removeEventListener('pagehide',feed.stop);document.removeEventListener('visibilitychange',visibility)}},[feed])
  const response=state.response;const s=response?.data
  const all=[...(s?.history??[]),...older].filter((v,i,rows)=>rows.findIndex(b=>b.sequence===v.sequence)===i).sort((a,b)=>b.sequence-a.sequence)
  async function more(){if(historyPending||!all.length)return;setHistoryPending(true);setHistoryError(false);const controller=new AbortController();historyController.current=controller
    try{const page=await auctionAPI.history(id,all[all.length-1]!.sequence,controller.signal);if(alive.current){setOlder(old=>[...old,...page.bids]);setHasOlder(page.has_more)}}catch{if(alive.current)setHistoryError(true)}finally{if(alive.current)setHistoryPending(false)}
  }
  return <section className="flex flex-col gap-6 pt-8 mt-2 border-t-2 border-ink" aria-labelledby={`auction-${id}`}>
    <h2 id={`auction-${id}`}>{b('heading')}</h2>
    {!s?<>{beforeBid}<p role="status">{b(state.connection==='loading'?'loading':'unavailable')}</p><Button variant="outline" type="button" onClick={()=>void feed.refresh()}>{b('retry')}</Button></>:<>
      <Badge tone="live">{b(s.auction_state)}</Badge>
      <p role="status">{state.stale?b('stale'):state.connection==='reconnecting'?b('reconnecting'):''}</p>
      <dl className="flex flex-col numeric"><div className="flex items-baseline gap-3 border-b-2 border-ink py-3"><dt className="mono-label">{b('current')}</dt><dd key={s.current_price_minor ?? 'none'} className={`ms-auto text-end font-mono ${s.current_price_minor===null?'text-base text-muted-foreground':'text-[1.75rem]'} motion-safe:animate-[confirmed-feedback_150ms_ease-out]`}>{s.current_price_minor===null?b('noBids'):formatCatalogMoney(s.current_price_minor, lang)}</dd></div><div className="flex items-baseline gap-3 border-b border-dashed border-ink/25 py-2"><dt className="mono-label">{b('minimum')}</dt><dd className="ms-auto text-end">{s.amount_limit_reached?b('amount_limit_reached'):formatCatalogMoney(s.next_minimum_minor, lang)}</dd></div><div className="flex items-baseline gap-3 border-b border-dashed border-ink/25 py-2"><dt className="mono-label">{b('openingPrice')}</dt><dd className="ms-auto text-end">{formatCatalogMoney(s.starting_price_minor, lang)}</dd></div><div className="flex items-baseline gap-3 border-b border-dashed border-ink/25 py-2"><dt className="mono-label">{b('increment')}</dt><dd className="ms-auto text-end">{formatCatalogMoney(s.bid_increment_minor, lang)}</dd></div><div className="flex items-baseline gap-3 border-b border-dashed border-ink/25 py-2"><dt className="mono-label">{b('count')}</dt><dd className="ms-auto text-end">{s.bid_count}</dd></div><div className="flex items-baseline gap-3 border-b border-dashed border-ink/25 py-2"><dt className="mono-label">{b('closes')}</dt><dd className="ms-auto text-end"><time dateTime={new Date(s.closes_at).toISOString()}>{new Intl.DateTimeFormat(lang==='vi'?'vi-VN':'en-US',{dateStyle:'medium',timeStyle:'short'}).format(s.closes_at)}</time></dd></div></dl>
      <div aria-live="polite" role="status">{s.result&&<p>{b(s.result.outcome)}{s.result.winner&&` · ${b('winner')}: ${s.result.winner.alias} · ${formatCatalogMoney(s.result.winner.amount_minor, lang)}`}</p>}</div>
      {beforeBid}
      <AccountStatus target={target}>{account.status==='signed-in'&&<BidForm key={account.profile.id} id={id} response={response} now={state.now} stale={state.stale} accept={feed.accept} refresh={feed.refresh}/>}</AccountStatus>
      <details className="border-t border-dashed border-ink/25 pt-3"><summary className="mono-label cursor-pointer min-h-11 flex items-center">{b('history')} ({all.length})</summary>
      {all.length===0?<p>{b('noBids')}</p>:<ol className="flex flex-col">{all.map(v=><li key={v.sequence} className="flex flex-wrap gap-x-4 gap-y-2 border-b border-ink/20 py-3"><span>#{v.sequence} · {v.alias}</span><strong className="ms-auto numeric">{formatCatalogMoney(v.amount_minor, lang)}</strong><time className="basis-full text-sm" dateTime={new Date(v.accepted_at).toISOString()}>{new Intl.DateTimeFormat(lang==='vi'?'vi-VN':'en-US',{dateStyle:'short',timeStyle:'medium'}).format(v.accepted_at)}</time></li>)}</ol>}
      {historyError&&<p role="alert">{b('unavailable')}</p>}
      {(hasOlder??s.history_has_more)&&<Button variant="outline" type="button" disabled={historyPending} onClick={()=>void more()}>{b('older')}</Button>}</details>
    </>}
    <p>{b('checkout')}</p>
  </section>
}

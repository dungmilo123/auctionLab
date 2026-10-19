import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type FormEvent } from 'react'
import { auctionAPI, AuctionError, type AuctionBid, type AuctionResponse, type Intent } from '@/api/auction'
import { AuctionFeed } from '@/auction/feed'
import { mayBid } from '@/auction/eligibility'
import { useParticipation } from '@/auction/useParticipation'
import { useAuctionCopy, auctionErrorKey, type AuctionKey } from '@/lib/auction-copy'
import { formatCatalogMoney, parseCatalogMoney } from '@/lib/catalog-money'
import { useAccount } from '@/account/AccountProvider'
import { AccountStatus } from '@/routes/account/AccountStatus'

function BidForm({ id, response, now, stale, accept, refresh }: { id:string;response:AuctionResponse|null;now:number;stale:boolean;accept:(s:AuctionResponse)=>void;refresh:()=>Promise<void> }) {
  const { b }=useAuctionCopy();const {store}=useAccount();const [value,setValue]=useState('');const [intent,setIntent]=useState<Intent|null>(null);const [sending,setSending]=useState(false);const [message,setMessage]=useState<AuctionKey|null>(null)
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
    let amount:number|null;try{amount=parseCatalogMoney(value)}catch{setMessage('invalid_amount');return};if(amount===null){setMessage('invalid_amount');return}
    const next={lot_id:id,amount_minor:amount,request_id:crypto.randomUUID()};pending.current=next;setIntent(next);void send(next)
  }
  return <div className="stack">
    <p>{b(state.status==='success'?state.data.status:state.status==='loading'?'identity':'privateUnavailable')}</p>
    {state.status==='error'&&<button className="site-button" type="button" onClick={retry}>{b('retry')}</button>}
    <form className="stack-small auction-form" onSubmit={submit}>
      <label htmlFor={`bid-${id}`}>{b('amount')}</label>
      <input id={`bid-${id}`} inputMode="decimal" autoComplete="off" value={value} readOnly={!eligible} aria-disabled={!eligible} onChange={e=>setValue(e.target.value)} aria-describedby={`bid-help-${id} bid-message-${id}`} />
      <p id={`bid-help-${id}`}>{b('help')}</p>
      <button className="site-button site-button-primary" type="submit" aria-disabled={!eligible}>{b(sending?'sending':'submit')}</button>
    </form>
    <p id={`bid-message-${id}`} role="status" aria-live="polite">{message?b(message):''}</p>
    {intent&&!sending&&<div className="stack-small"><p>{b('unknown')}</p><div className="actions"><button className="site-button" type="button" onClick={retry}>{b('reconcile')}</button><button className="site-button" type="button" onClick={()=>void send(intent)}>{b('sameRetry')}</button></div></div>}
  </div>
}
export default function AuctionPanel({ id,target }:{id:string;target:string}){
  const {b,lang}=useAuctionCopy();const {state:account}=useAccount();const feed=useMemo(()=>new AuctionFeed(id),[id]);const state=useSyncExternalStore(feed.subscribe,feed.get,feed.get)
  const [older,setOlder]=useState<AuctionBid[]>([]);const [historyPending,setHistoryPending]=useState(false);const [historyError,setHistoryError]=useState(false);const [hasOlder,setHasOlder]=useState<boolean|null>(null)
  const alive=useRef(true);const historyController=useRef<AbortController|null>(null)
  useEffect(()=>{alive.current=true;feed.start();const resume=()=>{if(!document.hidden){feed.stop();feed.start()}};const visibility=()=>{if(document.hidden)feed.stop();else resume()};window.addEventListener('focus',resume);window.addEventListener('pagehide',feed.stop);document.addEventListener('visibilitychange',visibility);return()=>{alive.current=false;historyController.current?.abort();feed.stop();window.removeEventListener('focus',resume);window.removeEventListener('pagehide',feed.stop);document.removeEventListener('visibilitychange',visibility)}},[feed])
  const response=state.response;const s=response?.data
  const all=[...(s?.history??[]),...older].filter((v,i,rows)=>rows.findIndex(b=>b.sequence===v.sequence)===i).sort((a,b)=>b.sequence-a.sequence)
  async function more(){if(historyPending||!all.length)return;setHistoryPending(true);setHistoryError(false);const controller=new AbortController();historyController.current=controller
    try{const page=await auctionAPI.history(id,all[all.length-1]!.sequence,controller.signal);if(alive.current){setOlder(old=>[...old,...page.bids]);setHasOlder(page.has_more)}}catch{if(alive.current)setHistoryError(true)}finally{if(alive.current)setHistoryPending(false)}
  }
  return <section className="bid-panel stack" aria-labelledby={`auction-${id}`}>
    <h2 id={`auction-${id}`}>{b('heading')}</h2>
    {!s?<><p role="status">{b(state.connection==='loading'?'loading':'unavailable')}</p><button type="button" className="site-button" onClick={()=>void feed.refresh()}>{b('retry')}</button></>:<>
      <p className="status-badge">{b(s.auction_state)}</p>
      <p role="status">{state.stale?b('stale'):state.connection==='reconnecting'?b('reconnecting'):''}</p>
      <dl className="metadata numeric"><div><dt>{b('openingPrice')}</dt><dd>{formatCatalogMoney(s.starting_price_minor)}</dd></div><div><dt>{b('increment')}</dt><dd>{formatCatalogMoney(s.bid_increment_minor)}</dd></div><div><dt>{b('current')}</dt><dd>{s.current_price_minor===null?b('noBids'):formatCatalogMoney(s.current_price_minor)}</dd></div><div><dt>{b('minimum')}</dt><dd>{s.amount_limit_reached?b('amount_limit_reached'):formatCatalogMoney(s.next_minimum_minor)}</dd></div><div><dt>{b('count')}</dt><dd>{s.bid_count}</dd></div><div><dt>{b('closes')}</dt><dd><time dateTime={new Date(s.closes_at).toISOString()}>{new Intl.DateTimeFormat(lang==='vi'?'vi-VN':'en-US',{dateStyle:'medium',timeStyle:'long'}).format(s.closes_at)}</time></dd></div></dl>
      <div aria-live="polite" role="status">{s.result&&<p>{b(s.result.outcome)}{s.result.winner&&` · ${b('winner')}: ${s.result.winner.alias} · ${formatCatalogMoney(s.result.winner.amount_minor)}`}</p>}</div>
      <AccountStatus target={target}>{account.status==='signed-in'&&<BidForm key={account.profile.id} id={id} response={response} now={state.now} stale={state.stale} accept={feed.accept} refresh={feed.refresh}/>}</AccountStatus>
      <h3>{b('history')}</h3>
      {all.length===0?<p>{b('noBids')}</p>:<ol className="auction-history">{all.map(v=><li key={v.sequence}><span>#{v.sequence} · {v.alias}</span><strong className="numeric">{formatCatalogMoney(v.amount_minor)}</strong><time dateTime={new Date(v.accepted_at).toISOString()}>{new Intl.DateTimeFormat(lang==='vi'?'vi-VN':'en-US',{dateStyle:'short',timeStyle:'medium'}).format(v.accepted_at)}</time></li>)}</ol>}
      {historyError&&<p role="alert">{b('unavailable')}</p>}
      {(hasOlder??s.history_has_more)&&<button type="button" className="site-button" disabled={historyPending} onClick={()=>void more()}>{b('older')}</button>}
    </>}
    <p>{b('checkout')}</p>
  </section>
}

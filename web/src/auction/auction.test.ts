import { afterEach, describe, expect, it, vi } from 'vitest'
import { auctionAPI, AuctionError, isAuctionSnapshot, mergeAuction, parseAuctionResponse, type AuctionResponse, type Participation } from '@/api/auction'
import { AuctionFeed } from './feed'
import { mayBid,filteredParticipation } from './eligibility'
import { startParticipationPoll } from './useParticipation'
import { AUCTION_COPY, auctionText } from '@/lib/auction-copy'
import { parseCatalogMoney } from '@/lib/catalog-money'
const base = () => ({lot_id:'l',revision:2,auction_state:'open',opens_at:100,closes_at:200,currency:'USD',starting_price_minor:0,bid_increment_minor:5,current_price_minor:null,next_minimum_minor:0,amount_limit_reached:false,bid_count:0,leader:null,history:[],history_has_more:false,result:null})
const sample = (revision=2,time=150):AuctionResponse => parseAuctionResponse({data:{...base(),revision},meta:{server_time:time}})
const own = (status:Participation['status']='not_participating'):Participation=>({lot_id:'l',slug:'lot',title:'Lot',status,latest:null,bids:[],current_price_minor:null,revision:2,checkout_available:false})
const deferred=<T>()=>{let resolve!:(v:T)=>void;let reject!:(v:unknown)=>void;const promise=new Promise<T>((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject}}
class FakeStream {
 handlers=new Map<string,EventListener>();closed=false;onerror:((event:Event)=>unknown)|null=null
 addEventListener(type:string,listener:EventListenerOrEventListenerObject){this.handlers.set(type,listener as EventListener)}
 close(){this.closed=true}
 send(type:string,data:unknown){this.handlers.get(type)?.({data:JSON.stringify(data)} as MessageEvent)}
}
afterEach(()=>{vi.useRealTimers();vi.unstubAllGlobals()})
describe('auction wire validation and monotonic state',()=>{
 it('rejects malformed and internally inconsistent execution',()=>{
   expect(isAuctionSnapshot(base())).toBe(true)
   for(const update of [{currency:'VND'},{revision:-1},{bid_count:1},{next_minimum_minor:null},{bid_increment_minor:0},{current_price_minor:0},{auction_state:'sold'},{history:[{}]},{closes_at:100},{revision:0},{amount_limit_reached:true},{auction_state:'anything'}])expect(isAuctionSnapshot({...base(),...update})).toBe(false)
   expect(()=>parseAuctionResponse({data:base(),meta:{server_time:'now'}})).toThrow()
 })
 it('discards lower revisions and older same-revision time while accepting fresh presentation',()=>{
   const current=sample(3,160);expect(mergeAuction(current,sample(2,170))).toBe(current);expect(mergeAuction(current,sample(3,150))).toBe(current)
   expect(mergeAuction(current,sample(3,170)).serverTime).toBe(170)
 })
 it('requires exact current execution and participation before submission',()=>{
   expect(mayBid(sample(),150,false,own(),false)).toBe(true)
   for(const [now,stale,p,pending] of [[99,false,own(),false],[200,false,own(),false],[150,true,own(),false],[150,false,null,false],[150,false,own('leading'),false],[150,false,{...own(),revision:1},false],[150,false,own(),true]] as const)expect(mayBid(sample(),now,stale,p,pending)).toBe(false)
 })
 it('parses exact cents including zero/jumps and refuses absent or fractional cents',()=>{expect(parseCatalogMoney('0.00')).toBe(0);expect(parseCatalogMoney('140.01')).toBe(14001);expect(parseCatalogMoney('')).toBeNull();for(const v of ['1.001','-1','1e3','10000000000.01'])expect(()=>parseCatalogMoney(v)).toThrow()})
 it('keeps closing in All and translates all labels',()=>{const rows=[{status:'leading'},{status:'closing'},{status:'won'}];expect(filteredParticipation(rows,'all')).toEqual(rows);expect(filteredParticipation(rows,'won')).toEqual([{status:'won'}]);for(const [en,vi]of Object.values(AUCTION_COPY)){expect(en.trim()).not.toBe('');expect(vi.trim()).not.toBe('')};expect(auctionText('vi','won')).toContain('thắng')})
})
describe('recoverable public transport',()=>{
 it('merges stream/HTTP, falls back every two seconds, detects stale data and cleans up',async()=>{
  vi.useFakeTimers();const pending=deferred<AuctionResponse>();const load=vi.fn().mockReturnValueOnce(pending.promise).mockRejectedValue(new Error('offline'));const streams:FakeStream[]=[]
  const feed=new AuctionFeed('l',load,()=>{const s=new FakeStream();streams.push(s);return s});feed.start()
  streams[0]!.send('snapshot',{data:{...base(),revision:4},meta:{server_time:150}});pending.resolve(sample(2,140));await Promise.resolve();expect(feed.get().response?.data.revision).toBe(4)
  streams[0]!.onerror?.({} as Event);await vi.advanceTimersByTimeAsync(10001);expect(load.mock.calls.length).toBeGreaterThan(4);expect(feed.get().stale).toBe(true)
  const last=streams.at(-1)!;last.send('snapshot',{data:{...base(),revision:5},meta:{server_time:160}});expect(feed.get().stale).toBe(false)
  feed.stop();const calls=load.mock.calls.length;await vi.advanceTimersByTimeAsync(20000);expect(load).toHaveBeenCalledTimes(calls);expect(streams.every(s=>s.closed)).toBe(true)
 })
 it('ignores obsolete responses after stop and malformed stream data',async()=>{
  vi.useFakeTimers();const d=deferred<AuctionResponse>();const stream=new FakeStream();const feed=new AuctionFeed('l',()=>d.promise,()=>stream);feed.start();stream.send('snapshot',{data:{...base(),bid_count:-1},meta:{server_time:150}});expect(feed.get().response).toBeNull();feed.stop();d.resolve(sample());await Promise.resolve();expect(feed.get().response).toBeNull()
 })
})
describe('private session lifetime and intent recovery',()=>{
 it('drops obsolete private responses and aborts on logout/unmount',async()=>{vi.useFakeTimers();const a=deferred<string>();const b=deferred<string>();let signal:AbortSignal|undefined;const load=vi.fn((s:AbortSignal)=>{signal=s;return load.mock.calls.length===1?a.promise:b.promise});const success=vi.fn();const p=startParticipationPoll(load,success,vi.fn(),()=>true);p.refresh();a.resolve('old account');await Promise.resolve();expect(success).not.toHaveBeenCalled();p.stop();expect(signal?.aborted).toBe(true);b.resolve('new but obsolete');await Promise.resolve();expect(success).not.toHaveBeenCalled()})
 it('does not turn failed private reads into an empty list and pauses hidden polls',async()=>{vi.useFakeTimers();const failure=vi.fn();const success=vi.fn();let visible=true;const load=vi.fn().mockRejectedValue(new AuctionError('unavailable'));const p=startParticipationPoll(load,success,failure,()=>visible);await Promise.resolve();expect(failure).toHaveBeenCalled();expect(success).not.toHaveBeenCalled();visible=false;await vi.advanceTimersByTimeAsync(6000);expect(load).toHaveBeenCalledTimes(1);p.stop()})
 it('allows a slow private read to finish instead of aborting it on every poll tick',async()=>{vi.useFakeTimers();const d=deferred<string>();const load=vi.fn(()=>d.promise);const success=vi.fn();const p=startParticipationPoll(load,success,vi.fn(),()=>true);await vi.advanceTimersByTimeAsync(3000);expect(load).toHaveBeenCalledTimes(1);d.resolve('confirmed participation');await Promise.resolve();expect(success).toHaveBeenCalledWith('confirmed participation');p.stop()})
 it('preserves exact intent IDs and distinguishes confirmed conflict from unknown transport',async()=>{
  const intent={lot_id:'l',amount_minor:0,request_id:'00000000-0000-0000-0000-000000000001'}
  const fetcher=vi.fn().mockRejectedValueOnce(new Error('response lost')).mockResolvedValue(new Response(JSON.stringify({data:{receipt:{lot_id:'l',bid:{sequence:1,amount_minor:0,accepted_at:150,request_id:intent.request_id},replay:true},snapshot:null},meta:{server_time:210}}),{status:200}))
  vi.stubGlobal('fetch',fetcher);await expect(auctionAPI.submit(intent)).rejects.toThrow('unavailable');const accepted=await auctionAPI.submit(intent);expect(accepted.receipt.replay).toBe(true);expect(fetcher.mock.calls.map(c=>JSON.parse(c[1].body).request_id)).toEqual([intent.request_id,intent.request_id])
 })
})

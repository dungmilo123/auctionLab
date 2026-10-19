import { auctionAPI, mergeAuction, parseAuctionResponse, type AuctionResponse } from '@/api/auction'
import { validInstant } from '@/api/public-catalog'
export type FeedState = { response: AuctionResponse | null; connection: 'loading' | 'live' | 'reconnecting' | 'unavailable'; stale: boolean; now: number }
type Stream = { addEventListener(type:string,listener:EventListener):void;close():void;onerror:((event:Event)=>unknown)|null }
export class AuctionFeed {
  private value: FeedState = { response: null, connection: 'loading', stale: true, now: 0 }
  private listeners = new Set<() => void>()
  private stream?: Stream; private timer?: ReturnType<typeof setInterval>; private reconnect?: ReturnType<typeof setTimeout>; private controller?: AbortController
  private lastFresh = -Infinity; private anchor = 0; private serverTime = 0; private stopped = true; private backoff = 1000; private pollAt = 0
  private id: string; private load: typeof auctionAPI.snapshot; private create: (url:string)=>Stream
  constructor(id:string,load=auctionAPI.snapshot,create:(url:string)=>Stream=url=>new EventSource(url)){this.id=id;this.load=load;this.create=create}
  get = () => this.value
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn) } }
  private set(v: FeedState) { this.value = v; this.listeners.forEach(f => f()) }
  accept = (next: AuctionResponse) => {
    if (this.stopped || next.data.lot_id !== this.id) return
    const response = mergeAuction(this.value.response, next)
    if (response !== next) return
    this.lastFresh = this.anchor = performance.now(); this.serverTime = next.serverTime
    this.set({ ...this.value, response, now: next.serverTime, stale: false })
  }
  refresh = async () => {
    if (this.stopped) return
    this.controller?.abort(); const controller = new AbortController(); this.controller = controller
    try { const response = await this.load(this.id, controller.signal); if (!this.stopped && !controller.signal.aborted) { this.accept(response); if (this.value.connection === 'loading' || this.value.connection === 'unavailable') this.set({ ...this.value, connection: 'reconnecting' }) } }
    catch { if (!this.stopped && !controller.signal.aborted) this.set({ ...this.value, connection: this.value.response ? 'reconnecting' : 'unavailable' }) }
  }
  private connect = () => {
    if (this.stopped) return
    this.stream?.close()
    let stream: Stream
    try { stream = this.create(auctionAPI.streamURL(this.id)) } catch { this.failed(); return }
    this.stream = stream
    stream.addEventListener('snapshot', ((event: MessageEvent) => {
      if (this.stopped || this.stream !== stream) return
      try { const next = parseAuctionResponse(JSON.parse(event.data)); if (next.data.lot_id !== this.id) throw new Error('lot'); this.accept(next); this.backoff = 1000; this.set({ ...this.value, connection: 'live' }) } catch { this.failed() }
    }) as EventListener)
    stream.addEventListener('heartbeat', ((event: MessageEvent) => {
      if (this.stopped || this.stream !== stream) return
      try { const b = JSON.parse(event.data) as { meta?: { server_time?: unknown } }; const t=b.meta?.server_time
        if (!validInstant(t) || t < this.serverTime) throw new Error('time')
        // A heartbeat cannot rehabilitate an already-stale snapshot on its own.
        if (this.value.stale) { void this.refresh(); return }
        this.lastFresh = this.anchor = performance.now(); this.serverTime = t; this.set({ ...this.value, now: t })
      } catch { this.failed() }
    }) as EventListener)
    stream.onerror = () => { if (this.stream === stream) this.failed() }
  }
  private failed() {
    if (this.stopped) return
    this.stream?.close(); this.stream = undefined; this.set({ ...this.value, connection: 'reconnecting' }); void this.refresh()
    clearTimeout(this.reconnect); this.reconnect = setTimeout(this.connect, this.backoff); this.backoff = Math.min(this.backoff * 2, 10000)
  }
  start = () => {
    this.stopped = false; this.set({...this.value,stale:true,connection:this.value.response?'reconnecting':'loading'}); void this.refresh(); this.connect()
    this.timer = setInterval(() => { if (this.stopped) return; const elapsed=performance.now(); this.set({ ...this.value, stale: elapsed - this.lastFresh >= 10000, now: this.serverTime + Math.max(0,elapsed-this.anchor) }); if (this.value.connection !== 'live' && elapsed-this.pollAt>=2000) {this.pollAt=elapsed;void this.refresh()} }, 250)
    return this.stop
  }
  stop = () => { this.stopped = true; this.stream?.close(); this.controller?.abort(); clearInterval(this.timer); clearTimeout(this.reconnect) }
}

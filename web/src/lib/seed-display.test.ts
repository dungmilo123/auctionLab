import { expect, it } from 'vitest'
import { displayEvent, displayLot, displayOrder, displaySummary } from './seed-display'
import type { PublicLot, PublicEvent } from '@/api/catalog'
import { sampleOrder } from '@/api/fulfillment.fixture'
const fulfillment = { shipping_enabled:true,pickup_enabled:true,shipping_fee_minor:1250,shipping_instructions:'Demo delivery: protective packaging; fee disclosed in USD.',pickup_instructions:'Demo pickup by arrangement; no real collection address.' }
const lot = { id:'demo-lot-01',title:'Folding travel camera',demo:true,description:'Illustrative vintage object for catalog preparation. This is not a real sale listing.',condition:'Demo illustration depicts general wear; physical condition and operation are unknown.',story:"Project-authored demo story: an imagined object from a collector's shelf. No provenance has been independently verified.",images:[{path:'/site-assets/catalog/lot-01.svg',alt:'Folding travel camera demo illustration'},{path:'/site-assets/catalog/lot-01-detail.svg',alt:'Folding travel camera illustrated detail'}],specifications:[{name:'Representation',value:'Project-authored SVG illustration'}],fulfillment,starting_price_minor:12550 } as PublicLot
it('adapts exact stock fields without mutating raw catalog, amounts or unknown facts',()=>{
 const raw=structuredClone(lot),out=displayLot(raw)
 expect(out.description).toBe('A vintage object selected for the collection.');expect(out.condition).toContain('not been assessed');expect(out.story).toBe('Provenance information is unavailable.')
 expect(out.images.map(i=>i.alt)).toEqual(['Folding travel camera','Folding travel camera, detail']);expect(out.specifications[0]!.value).toBe('Object study')
 expect(out.fulfillment.pickup_instructions).toContain('address is not provided');expect(out.fulfillment.shipping_fee_minor).toBe(1250);expect(out.starting_price_minor).toBe(12550);expect(raw).toEqual(lot)
})
it('preserves edited fields, unrelated media and non-demo listings',()=>{
 for(const raw of [{...lot,demo:false},{...lot,images:[{path:'/images/custom.svg',alt:lot.images[0]!.alt}]}])expect(displayLot(raw)).toBe(raw)
 const out=displayLot({...lot,description:'My authored demo story',fulfillment:{...fulfillment,pickup_instructions:'Meet at the agreed address.'}})
 expect(out.description).toBe('My authored demo story');expect(out.condition).not.toBe(lot.condition);expect(out.fulfillment.pickup_instructions).toBe('Meet at the agreed address.')
})
it('adapts only exact event boilerplate with recognized provenance and cover',()=>{
 const e={demo:true,cover_path:'/site-assets/catalog/event-1.svg',cover_alt:'Analog Memories demonstration illustration',theme:'Illustrative vintage collection',description:'Demonstration content. These illustrations are fictional; no authenticity or sale is asserted.',lots:[lot]} as PublicEvent
 expect(displayEvent(e).cover_alt).toBe('Analog Memories collection');expect(displayEvent(e).lots[0]!.condition).toContain('not been assessed');expect(displayEvent({...e,demo:false})).toEqual({...e,demo:false})
 expect(displayEvent({...e,theme:'My demo collection'}).theme).toBe('My demo collection')
})
it('adapts historical owner snapshots, summaries and exact instructions while preserving receipts and amounts',()=>{
 const raw=structuredClone(sampleOrder);raw.snapshot.condition=lot.condition;raw.snapshot.image=lot.images[0]!;raw.snapshot.fulfillment=fulfillment;raw.options[0]!.instructions=fulfillment.shipping_instructions;raw.total!.instructions=fulfillment.shipping_instructions
 const before=structuredClone(raw),out=displayOrder(raw)
 expect(out.snapshot.condition).toContain('not been assessed');expect(out.total!.instructions).toBe('Protective packaging; delivery fee disclosed in USD.');expect(out.receipt).toEqual(raw.receipt);expect(out.total!.total_minor).toBe(raw.total!.total_minor);expect(raw).toEqual(before)
 expect(displayOrder({...raw,snapshot:{...raw.snapshot,lot_demo:false}}).snapshot).toEqual({...raw.snapshot,lot_demo:false})
 const s={lot_demo:true,image:lot.images[0]!} as Parameters<typeof displaySummary>[0];expect(displaySummary(s).image.alt).toBe('Folding travel camera')
})

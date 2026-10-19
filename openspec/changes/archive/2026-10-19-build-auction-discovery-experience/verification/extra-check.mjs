const {chromium} = await import(process.env.PLAYWRIGHT_MODULE ?? 'playwright')
import fs from 'node:fs'
import assert from 'node:assert/strict'
const s=JSON.parse(fs.readFileSync(new URL('./fixture.json', import.meta.url),'utf8'));const b=await chromium.launch();const results=[]
for(const origin of ['http://127.0.0.1:5173','http://127.0.0.1:8080']){
 const p=await b.newPage();const raw=await (await p.request.get(origin+'/api/catalog/lots/'+s.custom)).json()
 for(const [name,fulfillment] of [['zero-free',{...raw.data.fulfillment}],['pickup-only',{shipping_enabled:false,pickup_enabled:true,shipping_fee_minor:null,shipping_instructions:'',pickup_instructions:'Collect at the workshop.'}]]){
  await p.route('**/api/catalog/lots/'+s.custom,r=>r.fulfill({json:{...raw,data:{...raw.data,starting_price_minor:0,fulfillment}}}))
  await p.goto(origin+'/lots/'+s.custom);await p.getByRole('heading',{name:'Condition',exact:true}).waitFor();const text=await p.locator('main').innerText();assert.ok(text.includes('Opening price: $0.00 USD'))
  if(name==='zero-free')assert.ok(text.includes('Shipping fee: $0.00 USD'));else{assert.ok(text.includes('Shipping unavailable'));assert.ok(!text.includes('Shipping fee:'));assert.ok(text.indexOf('Collect at the workshop.')<text.indexOf('Place bid'))}
  await p.unroute('**/api/catalog/lots/'+s.custom);results.push({origin,check:name,status:'passed'})
 }
 for(const slug of s.events){const e=await(await p.request.get(origin+'/api/catalog/events/'+slug)).json();await p.goto(origin+'/auctions/'+slug);await p.locator('.lot-card').first().waitFor();assert.equal(await p.locator('.lot-card').count(),e.data.lots.length);assert.deepEqual(await p.locator('.lot-card h3 a').evaluateAll(a=>a.map(e=>e.getAttribute('href'))),e.data.lots.sort((a,b)=>a.position-b.position).map(l=>'/lots/'+l.slug));assert.ok((await p.locator('main').innerText()).includes('Demonstration record'));results.push({origin,check:'ordered complete event '+slug,status:'passed'})}
 await p.close()
}
fs.writeFileSync(new URL('./extra-results.json',import.meta.url),JSON.stringify(results,null,2)+'\n');console.log('PASS zero-cent/free-shipping, pickup-only and complete ordered demo event checks in both builds');await b.close()

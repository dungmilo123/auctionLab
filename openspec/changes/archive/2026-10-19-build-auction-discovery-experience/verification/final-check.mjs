const {chromium} = await import(process.env.PLAYWRIGHT_MODULE ?? 'playwright')
import fs from 'node:fs'
import assert from 'node:assert/strict'
const s=JSON.parse(fs.readFileSync(new URL('./fixture.json', import.meta.url),'utf8')),b=await chromium.launch(),results=[]
for(const origin of ['http://127.0.0.1:5173','http://127.0.0.1:8080']){
 const p=await b.newPage();let previous
 for(const slug of [s.draftEvent,'missing-event']){await p.goto(origin+'/auctions/'+slug);await p.getByRole('heading',{name:'Record not found',exact:true}).waitFor();const text=await p.locator('main').innerText();if(previous)assert.equal(text,previous);previous=text}
 await p.addInitScript(()=>{Date.now=()=>1})
 await p.goto(origin+'/lots/'+s.custom);await p.getByRole('heading',{name:'Condition',exact:true}).waitFor();assert.ok(!(await p.locator('main').innerText()).includes('Demo illustration'));assert.equal(await p.locator('main .demo-marker').count(),0)
 const raw=await(await p.request.get(origin+'/api/catalog/lots/'+s.custom)).json();const expected=raw.meta.server_time<raw.data.opens_at?'Upcoming schedule':raw.meta.server_time<raw.data.closes_at?'Within planned window':'Planned window elapsed';await p.getByText(expected,{exact:true}).waitFor()
 const missing=[];p.on('response',r=>{if(r.status()>=400)missing.push(new URL(r.url()).pathname)})
 await p.goto(origin+'/');await p.locator('.lot-card').first().waitFor();await p.locator('.hero-art img').evaluate(img=>img.decode());assert.equal(missing.length,0)
 results.push({origin,checks:['identical missing/draft event recovery','incorrect wall clock ignored','non-demo authored condition','home illustration loaded'],status:'passed'});await p.close()
}
await b.close();fs.writeFileSync(new URL('./final-browser-results.json',import.meta.url),JSON.stringify(results,null,2)+'\n');console.log('PASS missing/draft events, incorrect device wall clock, non-demo disclosures and homepage asset in both builds')

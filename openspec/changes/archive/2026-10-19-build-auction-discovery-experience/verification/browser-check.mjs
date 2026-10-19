const { chromium } = await import(process.env.PLAYWRIGHT_MODULE ?? 'playwright')
import fs from 'node:fs'
import assert from 'node:assert/strict'
const state=JSON.parse(fs.readFileSync(new URL('./fixture.json', import.meta.url),'utf8'))
const out=import.meta.dirname
const results=[]
const browser=await chromium.launch()
async function check(name,fn){await fn();results.push({check:name,status:'passed'});fs.writeFileSync(out+'/browser-results.json',JSON.stringify(results,null,2)+'\n');console.log('PASS '+name)}
for(const origin of ['http://127.0.0.1:5173','http://127.0.0.1:8080']) {
 const mode=origin.endsWith('5173')?'vite':'embedded'
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'})
 await context.addInitScript(()=>{localStorage.setItem('auctionlab.player_token','EXISTING_GAME_TOKEN_SENTINEL');localStorage.setItem('auctionlab.lang','en')})
 const page=await context.newPage(),requests=[],sockets=[]
 page.on('request',r=>requests.push({url:r.url(),method:r.method()}));page.on('websocket',s=>sockets.push(s.url()))
 const goto=async(path)=>{await page.goto(origin+path);await page.locator('main .empty-state h2').filter({hasText:'Loading'}).waitFor({state:'hidden'});await page.waitForTimeout(600)}
 await check(mode+' home/event/lot journey and authored content',async()=>{
  await goto('/');assert.equal(await page.locator('main h1').textContent(),'Objects with a past. Stories with a future.')
  assert.equal(await page.locator('main section').filter({has:page.getByRole('heading',{name:'Featured events',exact:true})}).locator('.lot-card').count(),3)
  assert.ok(await page.locator('a[href="/lots?category=film-cameras"]').count());await page.screenshot({path:`${out}/${mode}-home-en.png`,fullPage:true})
  await goto('/auctions');assert.equal(await page.locator('.lot-card').count(),4)
  await page.locator(`main a[href="/auctions/${state.event}"]`).click();await page.waitForTimeout(600);assert.equal(await page.locator('h1').textContent(),'Collector study');assert.equal(await page.locator('.lot-card').count(),1);await page.screenshot({path:`${out}/${mode}-event-en.png`,fullPage:true})
  await page.locator(`main a[href="/lots/${state.custom}"]`).click();await page.waitForTimeout(600)
  assert.ok((await page.locator('h1').textContent()).includes('Collector camera'))
  assert.equal(await page.locator('main .demo-marker').count(),0)
  assert.ok((await page.locator('main').innerText()).includes('$125.50 USD'))
  assert.ok((await page.locator('main').innerText()).includes('$1.01 USD'))
  assert.ok((await page.locator('main').innerText()).includes('$0.00 USD'))
  assert.equal(await page.locator('button[disabled]').count(),1)
  assert.equal(await page.evaluate(()=>window.catalogInjected),undefined)
  assert.equal(await page.getByRole('heading',{name:'Supplied story',exact:true}).count(),0)
  const content=await page.locator('main').innerText();assert.ok(content.indexOf('Free insured delivery.')<content.indexOf('Place bid'))
  await page.screenshot({path:`${out}/${mode}-desktop-lot-en.png`,fullPage:true})
 })
 await check(mode+' gallery keyboard, selected state, missing image fallback',async()=>{
  const second=page.getByRole('button',{name:'Select image 2: Collector camera back view'})
  await second.focus();await page.keyboard.press('Enter');assert.equal(await second.getAttribute('aria-pressed'),'true')
  assert.equal(await page.locator('section[aria-label="Image gallery"] > figure img').getAttribute('alt'),'Collector camera back view')
  await page.route('**/images/discovery-second.svg',r=>r.fulfill({status:404,body:'missing'}));await page.reload();await page.waitForTimeout(600);await page.getByRole('button',{name:'Select image 2: Collector camera back view'}).click()
  await page.locator('section[aria-label="Image gallery"] > figure figcaption').waitFor();assert.ok(await page.getByRole('heading',{name:'Condition',exact:true}).isVisible());await page.unroute('**/images/discovery-second.svg')
 })
 await check(mode+' bilingual 320/375 mobile and 200 percent text enlargement',async()=>{
  for(const width of [320,375]){
   await page.setViewportSize({width,height:900});await page.getByRole('button',{name:'Tiếng Việt',exact:true}).click()
   assert.equal(await page.locator('html').getAttribute('lang'),'vi')
   assert.equal(await page.getByRole('button',{name:'Chọn ảnh 2: Collector camera back view'}).getAttribute('aria-pressed'),'true')
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
   await page.screenshot({path:`${out}/${mode}-${width}-lot-vi.png`,fullPage:true})
   await page.getByRole('button',{name:'English',exact:true}).click()
  }
  await page.addStyleTag({content:'html {font-size:200% !important}'});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:`${out}/${mode}-320-text200-en.png`,fullPage:true})
  await page.setViewportSize({width:1440,height:1000})
 })
 await check(mode+' native GET combined filters, sorting, refresh and history',async()=>{
  await goto('/lots?q=Collector&category=film-cameras&phase=all&sort=price-asc')
  assert.equal(await page.locator('input[name=q]').inputValue(),'Collector');assert.equal(await page.locator('select[name=category]').inputValue(),'film-cameras');assert.equal(await page.locator('.lot-card').count(),1)
  await page.getByRole('button',{name:'Tiếng Việt',exact:true}).click();assert.equal(await page.locator('input[name=q]').inputValue(),'Collector');assert.equal(await page.locator('select[name=sort]').inputValue(),'price-asc')
  await page.getByRole('button',{name:'English',exact:true}).click()
  await page.locator('input[name=q]').fill('nothing-matches');await page.getByRole('button',{name:'Apply filters',exact:true}).click();await page.waitForTimeout(600);assert.ok(page.url().includes('q=nothing-matches'));assert.equal(await page.getByRole('heading',{name:'No matching records',exact:true}).count(),1)
  await page.reload();await page.waitForTimeout(600);assert.equal(await page.locator('input[name=q]').inputValue(),'nothing-matches')
  await page.goBack();await page.waitForTimeout(600);assert.equal(await page.locator('input[name=q]').inputValue(),'Collector')
  await page.goForward();await page.waitForTimeout(600);assert.equal(await page.locator('input[name=q]').inputValue(),'nothing-matches')
  await page.locator('form a[href="/lots"]').click();await page.waitForTimeout(600);assert.equal(await page.locator('.lot-card').count(),19)
  for(const sort of ['closing-soon','price-asc','price-desc','title']){await page.locator('select[name=sort]').selectOption(sort);await page.getByRole('button',{name:'Apply filters',exact:true}).click();await page.waitForTimeout(600);assert.equal(await page.locator('select[name=sort]').inputValue(),sort)}
  const other=await context.newPage();await other.goto(origin+'/lots?q=Collector&category=film-cameras&phase=all&sort=title');await other.waitForLoadState('networkidle');assert.equal(await other.locator('.lot-card').count(),1);await other.close()
  await page.setViewportSize({width:320,height:900});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:`${out}/${mode}-320-index-en.png`,fullPage:true});await page.setViewportSize({width:1440,height:1000})
 })
 await check(mode+' event GET phase controls and identical draft/missing detail recovery',async()=>{
  for(const phase of ['all','window','upcoming','elapsed']){await goto('/auctions?phase='+phase);assert.equal(await page.locator('select[name=phase]').inputValue(),phase)}
  let previous=''
  for(const slug of [state.draft,'unknown-slug']){await goto('/lots/'+slug);const text=await page.locator('main').innerText();assert.ok(text.includes('Record not found'));if(previous)assert.equal(text,previous);previous=text}
  previous='';for(const slug of [state.draftEvent,'missing-event']){await goto('/auctions/'+slug);const text=await page.locator('main').innerText();assert.ok(text.includes('Record not found'));if(previous)assert.equal(text,previous);previous=text}
 })
 await check(mode+' loading, empty, unavailable, malformed success and retry',async()=>{
  let release;const gate=new Promise(r=>{release=r});await page.route('**/api/catalog/lots',async r=>{await gate;await r.continue()})
  await page.goto(origin+'/lots');await page.getByRole('heading',{name:'Loading published collection…',exact:true}).waitFor();release();await page.waitForTimeout(600);assert.equal(await page.locator('.lot-card').count(),19);await page.unroute('**/api/catalog/lots')
  await page.route('**/api/catalog/lots',r=>r.fulfill({json:{data:[],meta:{server_time:Date.now()}}}));await goto('/lots');assert.equal(await page.getByRole('heading',{name:'No published records yet',exact:true}).count(),1);await page.unroute('**/api/catalog/lots')
  for(const payload of [{status:503,json:{error:{code:'unavailable'}}},{status:200,contentType:'text/html',body:'<html>malformed</html>'}]){
   await page.route('**/api/catalog/lots',r=>r.fulfill(payload));await goto('/lots?q=Collector');assert.equal(await page.locator('.lot-card').count(),0);await page.getByRole('button',{name:'Try again',exact:true}).waitFor();await page.unroute('**/api/catalog/lots');await page.getByRole('button',{name:'Try again',exact:true}).click();await page.waitForTimeout(600);assert.equal(await page.locator('.lot-card').count(),1);assert.equal(await page.locator('input[name=q]').inputValue(),'Collector')
  }
 })
 await check(mode+' server clock boundaries, visibility resume failure and recovery',async()=>{
  const actual=await (await context.request.get(origin+'/api/catalog/lots/'+state.custom)).json()
  let sample=100;const record={...actual.data,opens_at:1500,closes_at:3500}
  await page.route('**/api/catalog/lots/'+state.custom,r=>r.fulfill({json:{data:record,meta:{server_time:sample}}}))
  await goto('/lots/'+state.custom);await page.getByText('Upcoming schedule',{exact:true}).waitFor();await page.getByText('Within planned window',{exact:true}).waitFor({timeout:5000});await page.getByText('Planned window elapsed',{exact:true}).waitFor({timeout:5000})
  await page.unroute('**/api/catalog/lots/'+state.custom)
  await page.route('**/api/catalog/lots/'+state.custom,r=>r.fulfill({status:503,json:{error:{code:'unavailable'}}}))
  await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));await page.getByRole('heading',{name:'Collection temporarily unavailable',exact:true}).waitFor()
  await page.unroute('**/api/catalog/lots/'+state.custom);await page.getByRole('button',{name:'Try again',exact:true}).click();await page.waitForTimeout(600);assert.ok((await page.locator('h1').textContent()).includes('Collector camera'))
 })
 await check(mode+' public-only reads with existing game token and disabled commerce',async()=>{
  assert.ok(sockets.every(url=>new URL(url).pathname!=='/ws'&&!new URL(url).pathname.startsWith('/ws/')))
  const api=requests.filter(r=>new URL(r.url).pathname.startsWith('/api/'))
  assert.ok(api.length>0);assert.ok(api.every(r=>r.method==='GET'&&new URL(r.url).pathname.startsWith('/api/catalog/')))
  assert.ok(requests.every(r=>!new URL(r.url).pathname.startsWith('/ws')))
  assert.equal(await page.evaluate(()=>localStorage.getItem('auctionlab.player_token')),'EXISTING_GAME_TOKEN_SENTINEL')
  const text=await page.locator('body').innerText();for(const forbidden of ['DISCOVERY_PRIVATE_SENTINEL','EVENT_PRIVATE_SENTINEL','EXISTING_GAME_TOKEN_SENTINEL','All objects shown are illustrations.'])assert.ok(!text.includes(forbidden))
  assert.equal(await page.locator('button[disabled]').count(),1)
  fs.writeFileSync(`${out}/${mode}-network.json`,JSON.stringify({requests:requests.map(r=>({path:new URL(r.url).pathname,method:r.method})),websockets:sockets},null,2)+'\n')
 })
 await context.close()
}
await browser.close();fs.writeFileSync(out+'/browser-results.json',JSON.stringify(results,null,2)+'\n')

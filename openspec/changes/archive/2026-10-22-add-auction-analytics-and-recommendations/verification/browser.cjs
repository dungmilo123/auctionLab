// Isolated local Chromium verification; no credentials are printed or captured.
const {chromium}=require('/home/milo/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright');
async function nativeScreenshot(page,file){const cdp=await page.context().newCDPSession(page);const m=await cdp.send('Page.getLayoutMetrics');const nativeZoom=await page.evaluate(()=>devicePixelRatio);const p=await cdp.send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:m.cssContentSize.width*nativeZoom,height:m.cssContentSize.height*nativeZoom,scale:1}});require('fs').writeFileSync(file,Buffer.from(p.data,'base64'));await cdp.detach()}
const fs=require('fs'),path=require('path'),assert=require('node:assert/strict');
const manifest=JSON.parse(fs.readFileSync('/tmp/auctionlab-change8-rehearsal.json'));
const out=__dirname;const results=[];
(async()=>{
 const extension='/tmp/auctionlab-change8-zoom-extension';fs.mkdirSync(extension,{recursive:true});fs.writeFileSync(path.join(extension,'manifest.json'),JSON.stringify({name:'Isolated browser zoom verification',version:'1.0',manifest_version:3,permissions:['tabs'],background:{service_worker:'worker.js'}}));fs.writeFileSync(path.join(extension,'worker.js'),'chrome.runtime.onInstalled.addListener(()=>{});');
 const browser=await chromium.launchPersistentContext('/tmp/auctionlab-change8-chromium',{headless:true,executablePath:'/home/milo/.cache/ms-playwright/chromium-1248/chrome-linux64/chrome',args:[`--disable-extensions-except=${extension}`,`--load-extension=${extension}`],viewport:{width:1280,height:900}});
 const worker=browser.serviceWorkers()[0]||await browser.waitForEvent('serviceworker');
 const publicPage=await browser.newPage();let sockets=0,external=0;browser.on('page',p=>p.on('websocket',ws=>{if(new URL(ws.url()).pathname==='/ws')sockets++}));
 for(const base of ['http://127.0.0.1:5173','http://127.0.0.1:8080']){
  const mode=base.endsWith('5173')?'vite':'embedded';
  await browser.addCookies([{name:'auction_admin',value:manifest.capability,url:base,httpOnly:true,sameSite:'Lax'}]);
  // Confirm cookie name via a deliberately unauthenticated request before cookie setup below.
  const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));page.on('websocket',ws=>{if(new URL(ws.url()).pathname==='/ws')sockets++});page.on('request',r=>{if(!new URL(r.url()).hostname.match(/^(127\.0\.0\.1|localhost)$/))external++});
  let response=await page.request.get(base+'/api/admin/analytics');if(response.status()===401){await browser.addCookies([{name:'auctionlab_admin',value:manifest.capability,url:base,httpOnly:true,sameSite:'Lax'}]);response=await page.request.get(base+'/api/admin/analytics')}
  assert.equal(response.status(),200);const report=await response.json();assert.equal(report.data.summary.bids,'6');assert.equal(report.data.summary.bidders,'2');assert.equal(report.data.summary.winning_minor,'35000');assert.equal(report.data.summary.simulated_paid_minor,'16500');
  await page.goto(base+'/admin/analytics?event=all&dataset=all');await page.getByRole('button',{name:'English',exact:true}).click();await page.getByRole('heading',{name:'Auction analytics',exact:true}).waitFor();await page.locator('.analytics-summary').waitFor();
  for(const lang of ['en','vi'])for(const width of [320,375,1280]){
   await page.setViewportSize({width,height:900});await page.getByRole('button',{name:lang==='en'?'English':'Tiếng Việt',exact:true}).click();await page.screenshot({path:path.join(out,`${mode}-analytics-${lang}-${width}.png`),fullPage:true});const layout=await page.evaluate(()=>({inner:innerWidth,width:document.documentElement.scrollWidth}));assert.ok(layout.width<=layout.inner+1,JSON.stringify(layout));results.push({mode,page:'analytics',lang,width,layout});
  }
  await page.setViewportSize({width:1280,height:900});await page.getByRole('button',{name:'English',exact:true}).click();await page.goto(base+'/lots/'+manifest.source);await page.getByRole('heading',{name:'Lots to explore',exact:true}).waitFor();await page.locator('.recommendations .lot-card').waitFor();assert.ok((await page.locator('.recommendations').innerText()).includes('Same category'));
  await page.goto(base+'/');await page.locator('.recommendations .lot-card').waitFor();assert.ok((await page.locator('.recommendations').innerText()).includes('Closing soon'));
  // Two independent commerce contexts use the real sign-in endpoint; no tokens in evidence.
  for(const actor of ['a','b']){
   const buyer=await browser.browser().newContext();const login=await buyer.request.post(base+'/api/auth/login',{headers:{'X-AuctionLab-Account':'1','Origin':base},data:{email:actor+'@rehearsal.test',password:manifest.password}});assert.equal(login.status(),200);
   const bp=await buyer.newPage();await bp.goto(base+'/');await bp.locator('.recommendations .lot-card').waitFor();await bp.waitForFunction(()=>document.querySelector('.recommendations')?.textContent.includes('Saved lots are excluded.'));const text=await bp.locator('.recommendations').innerText();assert.ok(text.includes(actor==='a'?'Category from your watchlist':'Closing soon'));
   const mine=await buyer.request.get(base+'/api/account/orders');assert.equal(mine.status(),200);const orders=await mine.json();assert.equal(orders.data.length,actor==='a'?2:1);await buyer.close();
  }
  await page.goto(base+'/admin/analytics');await page.locator('.analytics-summary').waitFor();await page.locator('input[name=event]').fill('analog-memories');await page.locator('select[name=dataset]').selectOption('demo');await page.getByRole('button',{name:'Apply filters',exact:true}).click();await page.locator('.analytics-summary').waitFor();assert.ok(page.url().includes('event=analog-memories'));assert.equal(await page.locator('select[name=dataset]').inputValue(),'demo');await page.goBack();await page.locator('.analytics-summary').waitFor();assert.equal(await page.locator('input[name=event]').inputValue(),'all');await page.goForward();await page.locator('.analytics-summary').waitFor();
  // Actual tab zoom through Chrome's extension API, independently measured.
  const tabId=await worker.evaluate(async url=>(await chrome.tabs.query({})).find(t=>t.url===url).id,page.url());await worker.evaluate(async id=>{await chrome.tabs.setZoom(id,2)},tabId);const zoom=await worker.evaluate(id=>chrome.tabs.getZoom(id),tabId);assert.equal(zoom,2);
  for(const lang of ['en','vi'])for(const route of ['/admin/analytics','/','/lots/'+manifest.source]){
   await page.goto(base+route);await page.getByRole('button',{name:lang==='en'?'English':'Tiếng Việt',exact:true}).click();if(route.startsWith('/admin'))await page.locator('.analytics-summary').waitFor();else await page.locator('.recommendations .lot-card').waitFor();const measured=await page.evaluate(()=>({inner:innerWidth,dpr:devicePixelRatio,scroll:document.documentElement.scrollWidth}));assert.ok(measured.scroll<=measured.inner+1);await nativeScreenshot(page,path.join(out,`${mode}-zoom200-${lang}-${route.startsWith('/admin')?'analytics':route==='/'?'home':'lot'}.png`));results.push({mode,page:route,lang,chromeZoom:await worker.evaluate(id=>chrome.tabs.getZoom(id),tabId),measured});
  }
  await worker.evaluate(id=>chrome.tabs.setZoom(id,1),tabId);
  assert.deepEqual(errors,[]);results.push({mode,reference:report.data.summary,consoleErrors:errors});await page.close();
 }
 assert.equal(sockets,0);assert.equal(external,0);fs.writeFileSync(path.join(out,'browser-results.json'),JSON.stringify({results,sockets,external},null,2));await publicPage.close();await browser.close();console.log('Vite/embedded reference, navigation, owner privacy, responsive layout and actual Chrome zoom checks passed');
})().catch(e=>{console.error(e.message);process.exit(1)});

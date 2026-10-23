const {chromium}=require('/home/milo/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright');
const fs=require('fs'),path=require('path'),assert=require('node:assert/strict');
const m=JSON.parse(fs.readFileSync('/tmp/auctionlab-change9-rehearsal.json')),out=__dirname;
(async()=>{
 const extension='/tmp/auction-polish-zoom';fs.mkdirSync(extension,{recursive:true});fs.writeFileSync(path.join(extension,'manifest.json'),JSON.stringify({name:'Isolated acceptance zoom',version:'1.0',manifest_version:3,permissions:['tabs'],background:{service_worker:'worker.js'}}));fs.writeFileSync(path.join(extension,'worker.js'),'chrome.runtime.onInstalled.addListener(()=>{});');
 const c=await chromium.launchPersistentContext('/tmp/auction-polish-matrix-profile-final',{headless:true,executablePath:'/home/milo/.cache/ms-playwright/chromium-1248/chrome-linux64/chrome',args:[`--disable-extensions-except=${extension}`,`--load-extension=${extension}`],viewport:{width:1280,height:900}});
 const worker=c.serviceWorkers()[0]||await c.waitForEvent('serviceworker');const p=await c.newPage();const rows=[],errors=[];let external=0,sockets=0;p.on('pageerror',e=>errors.push(e.message));p.on('websocket',ws=>{if(new URL(ws.url()).pathname==='/ws')sockets++});
 await c.route('**/*',async r=>{if(!['127.0.0.1','localhost'].includes(new URL(r.request().url()).hostname)){external++;return r.abort()}return r.continue()});
 async function sign(base,actor='a'){const r=await c.request.post(base+'/api/auth/login',{headers:{'X-AuctionLab-Account':'1',Origin:base},data:{email:actor+'@rehearsal.test',password:m.password}});assert.equal(r.status(),200)}
 async function language(lang){await p.locator('.language-trigger').click();await p.getByRole('menuitemradio',{name:lang==='en'?'English':'Tiếng Việt',exact:true}).click();await p.waitForFunction(l=>document.documentElement.lang===l,lang)}
 async function capture(base,route,lang,width,zoom=1,tag='ready'){
  if(process.argv.includes('--resume')){const old=JSON.parse(fs.readFileSync(path.join(out,'matrix.json')));const found=old.rows.find(r=>r.mode===(base.endsWith('5173')?'vite':'embedded')&&r.route===route&&r.lang===lang&&r.width===width&&r.chromeZoom===zoom&&r.state===tag);if(found){rows.push(found);return}}
  await p.goto(base+route);await p.locator('h1').waitFor();await p.waitForTimeout(180);await language(lang);await p.waitForTimeout(270);
  const actual=await p.evaluate(()=>({inner:innerWidth,scroll:document.documentElement.scrollWidth,dpr:devicePixelRatio,reduced:matchMedia('(prefers-reduced-motion: reduce)').matches}));assert.ok(actual.scroll<=actual.inner+1,JSON.stringify({route,lang,width,zoom,actual}));
  const name=`${base.endsWith('5173')?'vite':'embedded'}-${lang}-${width}-${zoom===2?'zoom200-':''}${tag}-${route.replace(/[^a-z0-9]/gi,'_')||'home'}`;
  await p.screenshot({path:path.join(out,'after',name+'.png'),fullPage:true});rows.push({mode:base.endsWith('5173')?'vite':'embedded',route,lang,width,chromeZoom:zoom,state:tag,actual});if(!process.argv.includes('--resume'))fs.writeFileSync(path.join(out,'matrix.json'),JSON.stringify({rows,errors,external,sockets},null,2));
 }
 fs.mkdirSync(path.join(out,'after'),{recursive:true});
 for(const base of ['http://127.0.0.1:5173','http://127.0.0.1:8080']){
  await c.addCookies([{name:'auction_admin',value:m.capability,url:base,httpOnly:true,sameSite:'Lax'}]);await sign(base);
  const orders=(await (await c.request.get(base+'/api/account/orders')).json()).data;const first=orders[0].id;
  const routes=['/','/auctions','/auctions/analog-memories','/lots','/lots/'+m.source,'/account','/account/watchlist','/account/bids','/account/wins','/account/orders','/account/orders/'+first,'/account/checkout/'+first,'/admin','/admin/lots','/admin/lots?edit=demo-lot-06','/admin/auctions','/admin/auctions?edit=demo-event-3','/admin/orders','/admin/orders?view='+first,'/admin/analytics','/missing'];
  for(const lang of ['en','vi'])for(const width of [320,375,1280]){await p.setViewportSize({width,height:900});for(const route of routes)await capture(base,route,lang,width)}
  // Real per-tab browser zoom; independently read Chrome's zoom and CSS viewport.
  await p.setViewportSize({width:1280,height:900});const id=await worker.evaluate(async url=>(await chrome.tabs.query({})).find(t=>t.url===url).id,p.url());await worker.evaluate(id=>chrome.tabs.setZoom(id,2),id);assert.equal(await worker.evaluate(id=>chrome.tabs.getZoom(id),id),2);
  await p.emulateMedia({reducedMotion:'reduce'});
  for(const lang of ['en','vi'])for(const route of routes)await capture(base,route,lang,1280,2);
  await worker.evaluate(id=>chrome.tabs.setZoom(id,1),id);await p.emulateMedia({reducedMotion:'no-preference'});
  // Signed-out auth forms, empty owned list and an unpaid checkout belong to distinct identities.
  await c.clearCookies({name:'auction_account'});for(const lang of ['en','vi'])for(const width of [320,375,1280]){await p.setViewportSize({width,height:900});for(const route of ['/account/login','/account/register','/account/watchlist'])await capture(base,route,lang,width,1,'signed-out')}
  await sign(base,'c');for(const lang of ['en','vi']){await p.setViewportSize({width:320,height:900});await capture(base,'/account/watchlist',lang,320,1,'empty')}
  await sign(base,'b');const wins=(await (await c.request.get(base+'/api/account/wins')).json()).data;
  for(const lang of ['en','vi'])for(const width of [320,375,1280]){await p.setViewportSize({width,height:900});await capture(base,'/account/checkout/'+wins[0].id,lang,width,1,'unpaid')}
  // Native GET restoration and new-tab semantics.
  await p.goto(base+'/lots?q=camera&category=film-cameras&sort=title');await p.locator('.lot-card').first().waitFor();const queryURL=p.url();await p.locator('.lot-card h3 a').first().click();await p.locator('.gallery-open').waitFor();await p.goBack();assert.equal(p.url(),queryURL);await p.goForward();await p.locator('.gallery-open').waitFor();const href=await p.locator('.site-nav a').first().getAttribute('href');const newtab=await c.newPage();await newtab.goto(base+href);await newtab.locator('h1').waitFor();await newtab.close();
 }
 await c.clearCookies();await p.goto('http://127.0.0.1:8080/admin');assert.equal(await p.locator('input[name=k]').count(),1);await p.screenshot({path:path.join(out,'after','gate.png'),fullPage:true});const invalid=await p.goto('http://127.0.0.1:8080/admin?k=invalid');assert.equal(invalid.status(),401);assert.match(invalid.headers()['cache-control'],/no-cache/);await p.screenshot({path:path.join(out,'after','gate-invalid.png'),fullPage:true});
 assert.deepEqual(errors,[]);assert.equal(external,0);assert.equal(sockets,0);fs.writeFileSync(path.join(out,'matrix.json'),JSON.stringify({rows,errors,external,sockets},null,2));await c.close();console.log('Matrix complete: '+rows.length+' bilingual route/viewport/zoom observations');
})().catch(e=>{console.error(e.stack);process.exit(1)});

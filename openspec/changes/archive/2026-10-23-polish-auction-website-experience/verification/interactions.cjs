const {chromium}=require('/home/milo/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright');
const fs=require('fs'),path=require('path'),assert=require('node:assert/strict');
const m=JSON.parse(fs.readFileSync('/tmp/auctionlab-change9-rehearsal.json')),out=__dirname;
(async()=>{
 const b=await chromium.launch({executablePath:'/home/milo/.cache/ms-playwright/chromium-1248/chrome-linux64/chrome'});const results=[];
 for(const base of ['http://127.0.0.1:5173','http://127.0.0.1:8080']){
  const c=await b.newContext({viewport:{width:1280,height:900},recordVideo:{dir:path.join(out,'recordings'),size:{width:1280,height:900}}});const p=await c.newPage();let views=0,writes=0,listReads=0;const errors=[];
  p.on('pageerror',e=>errors.push(e.message));p.on('request',r=>{const u=new URL(r.url());if(u.pathname==='/api/catalog/lot-views'&&r.method()==='POST')views++;if(r.method()!=='GET'&&u.pathname!=='/api/catalog/lot-views')writes++;if(u.pathname==='/api/account/watchlist'&&r.method()==='GET')listReads++});
  await c.request.post(base+'/api/auth/login',{headers:{'X-AuctionLab-Account':'1',Origin:base},data:{email:'a@rehearsal.test',password:m.password}});
  await p.goto(base+'/lots/'+m.source);await p.locator('.gallery-open').waitFor();await p.locator('.watch-action button[aria-pressed]').waitFor();const initialViews=views,initialWrites=writes,initialReads=listReads;assert.equal(initialViews,1);
  await p.locator('.gallery-thumbnails button').nth(1).click();await p.locator('.gallery-open').click();await p.getByRole('dialog').waitFor();await p.getByRole('button',{name:'Previous image',exact:true}).click();assert.equal(await p.locator('.gallery-thumbnails button').first().getAttribute('aria-pressed'),'true');
  await p.keyboard.press('Escape');await p.getByRole('dialog').waitFor({state:'hidden'});assert.equal(await p.locator('.gallery-open').evaluate(e=>e===document.activeElement),true);
  await p.locator('.language-trigger').focus();await p.keyboard.press('Enter');await p.getByRole('menuitemradio',{name:'Tiếng Việt',exact:true}).click();await p.waitForFunction(()=>document.documentElement.lang==='vi');await p.locator('.language-trigger').click();await p.getByRole('menuitemradio',{name:'English',exact:true}).click();await p.locator('.language-trigger').click();await p.getByRole('menuitemradio',{name:'Tiếng Việt',exact:true}).click();
  assert.equal(await p.locator('.gallery-thumbnails button').first().getAttribute('aria-pressed'),'true');await p.locator('.language-trigger').click();await p.keyboard.press('Escape');await p.getByRole('menu').waitFor({state:'hidden'});assert.equal(await p.locator('.language-trigger').evaluate(e=>e===document.activeElement),true);
  assert.equal(views,initialViews);assert.equal(writes,initialWrites);assert.equal(listReads,1);
  await p.locator('.gallery-open').click();await p.emulateMedia({reducedMotion:'reduce'});await p.waitForFunction(()=>document.querySelector('.website').dataset.reducedMotion==='true');assert.equal(await p.locator('.inspection-dialog').evaluate(e=>getComputedStyle(e).animationName),'none');await p.keyboard.press('Escape');
  await p.emulateMedia({reducedMotion:'no-preference'});await p.waitForFunction(()=>document.querySelector('.website').dataset.reducedMotion==='false');
  await p.setViewportSize({width:320,height:900});await p.getByRole('button',{name:'Điều hướng',exact:true}).click();await p.locator('#mobile-navigation').waitFor();await p.keyboard.press('Escape');await p.locator('#mobile-navigation').waitFor({state:'hidden'});
  await p.locator('.header-search-trigger').click();await p.locator('#site-search').fill('camera');await p.locator('#header-search button[type=submit]').click();await p.waitForURL('**/lots?q=camera');await p.locator('.filter-trigger').click();await p.locator('select[name=category]').waitFor({state:'visible'});await p.locator('select[name=category]').selectOption('film-cameras');await p.locator('.filter-trigger').press('Escape');await p.locator('.filter-trigger').click();assert.equal(await p.locator('select[name=category]').inputValue(),'film-cameras');
  assert.ok(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  results.push({base,languageAndGalleryPreserved:true,focusRestored:true,extraViews:views-initialViews,initialLotViews:initialViews,initialWatchlistReads:initialReads,commerceWritesDuringSelection:writes-initialWrites,runtimeReducedMotion:true,mobileSearchAndFilters:true,errors});assert.deepEqual(errors,[]);
  await c.close();await p.video().saveAs(path.join(out,'recordings',(base.endsWith('5173')?'vite':'embedded')+'-interactions.webm'));
 }
 await b.close();fs.writeFileSync(path.join(out,'interactions.json'),JSON.stringify(results,null,2));console.log('Language/gallery/membership/focus/runtime-motion/mobile GET checks passed');
})().catch(e=>{console.error(e.stack);process.exit(1)});

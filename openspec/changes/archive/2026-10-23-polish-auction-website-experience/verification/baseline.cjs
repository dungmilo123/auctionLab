const {chromium}=require('/home/milo/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright');
const fs=require('fs'),path=require('path');
const out=__dirname,m=JSON.parse(fs.readFileSync('/tmp/auctionlab-change9-rehearsal.json'));
(async()=>{
 const b=await chromium.launch({headless:true,executablePath:'/home/milo/.cache/ms-playwright/chromium-1248/chrome-linux64/chrome'});
 const c=await b.newContext({viewport:{width:1280,height:900}});
 const base='http://127.0.0.1:8080';
 await c.addCookies([{name:'auction_admin',value:m.capability,url:base,httpOnly:true,sameSite:'Lax'}]);
 await c.request.post(base+'/api/auth/login',{headers:{'X-AuctionLab-Account':'1','Origin':base},data:{email:'a@rehearsal.test',password:m.password}});
 let r=await c.request.get(base+'/api/account/orders');let orders=(await r.json()).data;
 let rows=[],requests=[];const p=await c.newPage();p.on('request',r=>requests.push({method:r.method(),path:new URL(r.url()).pathname}));
 const routes=['/','/auctions','/auctions/analog-memories','/lots','/lots/'+m.source,'/account','/account/login','/account/register','/account/watchlist','/account/bids','/account/wins','/account/orders','/account/orders/'+orders[0].id,'/account/checkout/'+orders[0].id,'/admin','/admin/lots','/admin/auctions','/admin/orders','/admin/analytics','/missing'];
 fs.mkdirSync(path.join(out,'before'),{recursive:true});
 for(const width of [1280,375]){
  await p.setViewportSize({width,height:900});
  for(const route of routes){let start=Date.now();await p.goto(base+route);await p.locator('h1').waitFor();await p.waitForTimeout(350);await p.screenshot({path:path.join(out,'before',`${width}-${route.replace(/[^a-z0-9]/gi,'_')||'home'}.png`),fullPage:true});rows.push({route,width,language:'en',elapsed_ms:Date.now()-start});}
 }
 await c.clearCookies();await p.goto(base+'/admin');await p.screenshot({path:path.join(out,'before','gate.png'),fullPage:true});
 const cd=await c.newCDPSession(p);await cd.send('Network.enable');await cd.send('Network.emulateNetworkConditions',{offline:false,latency:150,downloadThroughput:200000,uploadThroughput:100000});await cd.send('Emulation.setCPUThrottlingRate',{rate:4});
 await p.goto(base+'/lots/'+m.source);await p.locator('.gallery-thumbnails').waitFor();const start=Date.now();await p.locator('.gallery-thumbnails button').nth(1).click();await p.screenshot({path:path.join(out,'before','throttled-gallery.png')});
 rows.push({trace:'gallery selection',latency_ms:150,download_bytes_per_second:200000,cpu_slowdown:4,action_ms:Date.now()-start});
 fs.writeFileSync(path.join(out,'baseline.json'),JSON.stringify({fixture:m.schema,rows,requests},null,2));await b.close();console.log('Baseline captured: '+rows.length+' route/trace observations');
})().catch(e=>{console.error(e.message);process.exit(1)});

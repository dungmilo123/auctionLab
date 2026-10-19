// Change-local acceptance driver; Playwright is installed in /tmp, not a project dependency.
import assert from 'node:assert/strict'
import { mkdtemp, mkdir, writeFile, readFile, readdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { randomBytes } from 'node:crypto'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { fileURLToPath } from 'node:url'
const { chromium } = await import(process.env.AUCTIONLAB_PLAYWRIGHT ?? '/tmp/auctionlab-browser/node_modules/playwright/index.mjs')
const output = fileURLToPath(new URL('.', import.meta.url))
await mkdir(join(output, 'screenshots'), { recursive: true })
const temp = await mkdtemp(join(tmpdir(), 'auctionlab-foundation-'))
const origin = 'http://127.0.0.1:18281', dev = 'http://127.0.0.1:5173'
const key = randomBytes(32).toString('hex')
const server = spawn(resolve('bin/auctionlab'), ['--port', '18281'], {
  env: { ...process.env, DB_PATH: join(temp, 'acceptance.db'), ADMIN_KEY: key, IMAGES_DIR: join(temp, 'images'), PUBLIC_URL: '' }, stdio: 'ignore',
})
let browser
const results = { runAt: new Date().toISOString(), browser: '', routes: [], matrix: [], observations: [], contrast: [], requests: [], errors: [] }
const note = text => { results.observations.push(text); console.log(text) }
const routes = [
  ['/', 'Objects with a past.', 'storefront'], ['/auctions', 'Auctions', 'storefront'],
  ['/auctions/analog-memories', 'Auction preview', 'storefront'], ['/auctions/arbitrary', 'Auction preview', 'storefront'],
  ['/lots', 'Lots', 'storefront'], ['/lots/sample-camera', 'Lot preview', 'storefront'], ['/lots/arbitrary', 'Lot preview', 'storefront'],
  ['/account', 'Account', 'account'], ['/account/watchlist', 'Watchlist', 'account'],
  ['/account/bids', 'My bids', 'account'], ['/account/orders', 'Orders', 'account'],
  ['/admin', 'Administration', 'admin'], ['/admin/auctions', 'Manage auctions', 'admin'],
  ['/admin/lots', 'Manage lots', 'admin'], ['/admin/orders', 'Manage orders', 'admin'],
  ['/unknown-page', '404', 'storefront'], ['/admin/unknown-page', '404', 'storefront'],
  ['/screen', '404', 'storefront'], ['/screen/anything/deep', '404', 'storefront'],
]
async function ready(page, url) {
  const response = await page.goto(url)
  await page.locator('h1').waitFor()
  await page.evaluate(() => document.fonts.ready)
  return response
}
async function setLang(page, lang) {
  await page.getByRole('button', { name: lang === 'en' ? 'English' : 'Tiếng Việt', exact: true }).click()
  await page.waitForFunction(lang => document.documentElement.lang === lang, lang)
}
async function layoutCheck(page) {
  return page.evaluate(() => {
    const visible = e => { const r = e.getBoundingClientRect(); return r.width && r.height && getComputedStyle(e).visibility !== 'hidden' }
    const controls = [...document.querySelectorAll('a,button,input')].filter(e => visible(e) && !e.classList.contains('skip-link'))
    return {
      width: innerWidth, scrollWidth: document.documentElement.scrollWidth,
      smallTargets: controls.filter(e => { const r = e.getBoundingClientRect(); return r.width < 43.9 || r.height < 43.9 }).map(e => ({ tag: e.tagName, text: e.textContent, width: e.getBoundingClientRect().width, height: e.getBoundingClientRect().height })),
      h1Count: document.querySelectorAll('h1').length,
      clipped: [...document.querySelectorAll('main *,header *,footer *')].filter(e => visible(e) && e.getBoundingClientRect().right > innerWidth + 1).map(e => e.tagName + '.' + e.className).slice(0, 8),
    }
  })
}
async function screenshot(page, filename) { await page.screenshot({ path: join(output, 'screenshots', filename), fullPage: true }) }
try {
  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(origin + '/api/health'); if (r.ok) break } catch {}
    if (server.exitCode !== null) throw Error('Temporary server exited before readiness')
    await new Promise(r => setTimeout(r, 100))
  }
  browser = await chromium.launch({ headless: true })
  results.browser = browser.version()
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: 'reduce' })
  const page = await context.newPage()
  const sockets = [], business = [], external = []
  page.on('websocket', ws => sockets.push(new URL(ws.url()).pathname))
  page.on('pageerror', e => results.errors.push(e.message))
  context.on('request', request => {
    const url = new URL(request.url())
    if (url.protocol === 'data:') return
    if (url.origin !== origin) external.push(url.origin)
    if (/^\/(api|ws|images)(\/|$)/.test(url.pathname) || request.method() !== 'GET') business.push(url.pathname)
    results.requests.push({ path: url.pathname, method: request.method() })
  })
  // The key remains in memory only. Observe both missing/wrong variants before authorizing.
  for (const path of ['/admin', '/admin/lots']) {
    let response = await ready(page, origin + path)
    assert.equal(response.status(), 200); assert.equal(await page.locator('#k').count(), 1); assert.equal(await page.locator('#root').count(), 0)
    response = await ready(page, origin + path + '?k=invalid-preview-key')
    assert.equal(response.status(), 401); assert.equal(await page.locator('#k[aria-invalid="true"]').count(), 1)
    assert.equal(await page.locator('#k').inputValue(), '')
    assert(!(await page.content()).includes('invalid-preview-key'))
    assert.equal(await page.locator('#gate-error[role="alert"]').count(), 1)
  }
  await page.keyboard.press('Tab'); assert.equal(await page.locator('#k').evaluate(e => e === document.activeElement), true)
  await page.keyboard.press('Tab'); assert.equal(await page.locator('button').evaluate(e => e === document.activeElement), true)
  await screenshot(page, 'gate-error-1280.png')
  const authResponse = await context.request.get(origin + '/admin/lots?k=' + key, { maxRedirects: 0 })
  assert.equal(authResponse.status(), 303); assert.equal(authResponse.headers().location, '/admin')
  const cookies = await context.cookies(origin)
  const cap = cookies.find(c => c.name === 'auction_admin')
  assert(cap && cap.httpOnly && cap.sameSite === 'Lax' && cap.path === '/')
  assert.equal(cap.value, key)
  await page.goto(origin + '/admin/lots?k=' + key)
  assert.equal(page.url(), origin + '/admin')
  assert(!page.url().includes('?'))
  note('Go gate: no-key 200 and wrong-key 401 on /admin and /admin/lots; no SPA leakage/key echo; correct key 303 to literal /admin, HttpOnly SameSite=Lax cookie, key removed; keyboard and nested refresh pass.')
  // Direct loads and refresh, exact headings/layouts, every displayed link resolves.
  for (const [path, heading, layout] of routes) {
    const response = await ready(page, origin + path)
    assert.equal(response.status(), 200); assert.equal(response.headers()['cache-control'], 'no-cache')
    assert((await page.locator('h1').innerText()).startsWith(heading), path)
    assert.equal(await page.locator('.context-nav').count(), layout === 'storefront' ? 0 : 1)
    assert.equal(await page.locator('.preview-notice').count(), 1)
    assert.equal(await page.locator('h1').count(), 1)
    assert.equal(await page.locator('a[href*="?k="]').count(), 0)
    assert.equal(await page.locator('form').count(), 0)
    const before = await page.locator('h1').innerText()
    await page.reload(); await page.locator('h1').waitFor(); assert.equal(await page.locator('h1').innerText(), before)
    for (const href of await page.locator('a').evaluateAll(els => els.map(e => e.getAttribute('href')))) {
      assert(href === '#main' || routes.some(([r]) => r === href) || ['/lots/sample-clock', '/lots/sample-vinyl'].includes(href), 'Unexpected displayed link')
    }
    results.routes.push({ origin: 'Go', path, heading: before, layout, direct: 'pass', refresh: 'pass', cache: 'no-cache' })
  }
  // Real anchors: click, Back/Forward, and a new tab from the actual link.
  await ready(page, origin + '/')
  await page.locator('header a[href="/lots"]').click(); await page.waitForURL(origin + '/lots')
  assert.equal(await page.locator('header a[aria-current="page"]').innerText(), 'Lots')
  await page.goBack(); await page.waitForURL(origin + '/')
  await page.goForward(); await page.waitForURL(origin + '/lots')
  const popupPromise = context.waitForEvent('page')
  await page.locator('.lot-card a').first().click({ button: 'middle' })
  const popup = await popupPromise; await popup.waitForLoadState(); assert.equal(new URL(popup.url()).pathname, '/lots/sample-camera'); await popup.close()
  await ready(page, origin + '/lots')
  await page.keyboard.press('Tab'); assert.equal(await page.locator('.skip-link').evaluate(e => e === document.activeElement), true)
  const focus = await page.locator('.skip-link').evaluate(e => ({ outline: getComputedStyle(e).outlineColor, width: getComputedStyle(e).outlineWidth }))
  assert.equal(focus.width, '2px')
  await page.keyboard.press('Enter'); assert.equal(await page.locator('main').evaluate(e => e === document.activeElement), true)
  const invalid = page.locator('#invalid-example'); assert.equal(await invalid.getAttribute('aria-invalid'), 'true')
  assert.equal(await invalid.getAttribute('aria-describedby'), 'invalid-example-help invalid-example-error')
  assert.equal(await page.locator('label[for="invalid-example"]').count(), 1)
  assert.equal(await page.locator('button:disabled').count(), 1)
  await page.locator('#search-example').fill('Illustrative sample only')
  await page.locator('#search-example').press('Enter')
  assert.equal(new URL(page.url()).pathname, '/lots')
  await ready(page,origin+'/unknown-page');await page.locator('main a[href="/"]').click();await page.waitForURL(origin+'/')
  note('Go direct load/refresh: 19 route cases; exact layouts/headings, native links, active states, Back/Forward/new-tab, skip-link focus and label/help/error wiring pass.')
  // Both languages, every layout, four widths, text-only 200% scaling and reduced motion.
  const matrixPaths = ['/', '/lots', '/account/watchlist', '/admin/lots', '/unknown-page']
  for (const lang of ['en', 'vi']) {
    await setLang(page, lang)
    for (const width of [320, 375, 768, 1280]) {
      await page.setViewportSize({ width, height: 900 })
      for (const path of matrixPaths) for (const zoom of [100, 200]) {
        await ready(page, origin + path)
        if (zoom === 200) await page.evaluate(() => document.documentElement.style.fontSize = '200%')
        const checked = await layoutCheck(page)
        assert(checked.scrollWidth <= width + 1, `Overflow ${path} ${lang} ${width} ${zoom}: ${checked.scrollWidth}`)
        assert.equal(checked.smallTargets.length, 0, `Small targets ${path} ${lang} ${width} ${zoom}: ${JSON.stringify(checked.smallTargets)}`)
        assert.equal(checked.h1Count, 1)
        assert.equal(await page.locator('html').getAttribute('lang'), lang)
        const motion = await page.locator('.site-button').first().evaluate(e => getComputedStyle(e).transitionDuration)
        assert.equal(motion, '0s')
        results.matrix.push({ path, lang, width, textZoom: zoom, reducedMotion: true, ...checked })
        if (zoom === 100 && ((width === 1280 && lang === 'en') || (width === 320 && lang === 'vi'))) {
          await screenshot(page, `${path === '/' ? 'storefront' : path.slice(1).replaceAll('/', '-')}-${lang}-${width}.png`)
        }
        if (zoom === 200 && width === 375 && path === '/lots' && lang === 'vi') await screenshot(page, 'lots-vi-375-text200.png')
      }
    }
  }
  // Separate unauthenticated context for the self-contained, always-bilingual Go gate.
  const gateContext = await browser.newContext()
  const gatePage = await gateContext.newPage()
  for (const width of [320, 375, 768, 1280]) for (const zoom of [100, 200]) {
    await gatePage.setViewportSize({ width, height: 900 }); await gatePage.emulateMedia({ reducedMotion: 'reduce' })
    await ready(gatePage, origin + '/admin?k=invalid-preview-key')
    if (zoom === 200) await gatePage.evaluate(() => document.documentElement.style.fontSize = '200%')
    const checked = await layoutCheck(gatePage)
    assert(checked.scrollWidth <= width + 1, 'Gate overflow'); assert.equal(checked.smallTargets.length, 0, 'Gate target')
    results.matrix.push({ path: '/admin (gate)', lang: 'bilingual', width, textZoom: zoom, reducedMotion: true, ...checked })
    if (width === 375 && zoom === 100) await screenshot(gatePage, 'gate-bilingual-375.png')
  }
  results.gateContrast = await gatePage.evaluate(() => {
    const lum = color => color.match(/[\d.]+/g).slice(0,3).map(Number).map(x=>x/255).reduce((sum,x,i)=>sum+[.2126,.7152,.0722][i]*(x<=.04045?x/12.92:((x+.055)/1.055)**2.4),0)
    return ['h1','p','.vi','.error','.error .vi','label','input','button','button .vi'].map(selector => {
      const e=document.querySelector(selector),style=getComputedStyle(e)
      let ancestor=e,bg
      do { bg=getComputedStyle(ancestor).backgroundColor;ancestor=ancestor.parentElement } while(bg==='rgba(0, 0, 0, 0)' && ancestor)
      const a=lum(style.color),b=lum(bg)
      return {selector,foreground:style.color,background:bg,ratio:(Math.max(a,b)+.05)/(Math.min(a,b)+.05)}
    })
  })
  for(const pair of results.gateContrast) assert(pair.ratio>=4.5,'Gate contrast')
  await gateContext.close()
  // Traverse each representative website entirely by keyboard in each language.
  let keyboardChecks = 0
  for (const lang of ['en','vi']) {
    await setLang(page,lang)
    for (const path of matrixPaths) {
      await ready(page,origin+path)
      const targetCount = await page.locator('a[href],button:not(:disabled),input:not(:disabled)').count()
      for(let i=0;i<targetCount;i++) {
        await page.keyboard.press('Tab')
        const focus = await page.evaluate(() => { const e=document.activeElement,c=getComputedStyle(e),r=e.getBoundingClientRect();return {tag:e.tagName,outline:c.outlineWidth,color:c.outlineColor,visible:r.width>0&&r.height>0} })
        assert(['A','BUTTON','INPUT'].includes(focus.tag) && focus.visible && focus.outline==='2px','Keyboard focus')
        keyboardChecks++
      }
    }
  }
  results.keyboardChecks = keyboardChecks
  note('Responsive matrix: 80 website cases + 8 bilingual gate cases; 320/375/768/1280px, 100/200% text size, both website languages, reduced motion. No horizontal overflow or sub-44px targets.')
  // Measure actual rendered text, controls, selected state, statuses, focus and hover.
  await page.setViewportSize({ width: 1280, height: 900 }); await ready(page, origin + '/lots'); await setLang(page, 'en')
  const computed = await page.evaluate(() => {
    const lum = color => { const v = color.match(/[\d.]+/g).slice(0, 3).map(Number).map(x => x / 255); return v.reduce((s, x, i) => s + [.2126, .7152, .0722][i] * (x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4), 0) }
    const contrast = (a,b) => { const x = lum(a), y = lum(b); return (Math.max(x,y)+.05)/(Math.min(x,y)+.05) }
    const background = e => { while(e) { const bg = getComputedStyle(e).backgroundColor; if(bg !== 'rgba(0, 0, 0, 0)' && bg !== 'transparent') return bg; e=e.parentElement } return 'rgb(255, 255, 255)' }
    const selectors = ['h1', '.muted', '.preview-notice', '.status-upcoming', '.status-live', '.status-ended', '.status-sold', '.status-unavailable', '.field-error', '#search-example', '#invalid-example', 'button:disabled', '.language-control [aria-pressed=true]', '.site-button-outline']
    return selectors.map(selector => { const e = document.querySelector(selector), c = getComputedStyle(e), bg = background(e); return { selector, foreground: c.color, background: bg, ratio: contrast(c.color,bg), border: c.borderColor, borderRatio: contrast(c.borderColor,bg), font: c.fontFamily, opacity: c.opacity } })
  })
  for(const pair of computed) assert(pair.ratio >= 4.5, `Text contrast ${pair.selector}: ${pair.ratio}`)
  for(const pair of computed.filter(x => ['#search-example','#invalid-example','.site-button-outline'].includes(x.selector))) assert(pair.borderRatio >= 3)
  results.contrast.push(...computed)
  await ready(page, origin + '/')
  const action = page.locator('main .site-button-primary').first()
  const primaryColors = await action.evaluate(e => [getComputedStyle(e).color,getComputedStyle(e).backgroundColor])
  await action.hover()
  const hoverColors = await action.evaluate(e => [getComputedStyle(e).color,getComputedStyle(e).backgroundColor])
  assert.deepEqual(primaryColors, ['rgb(246, 241, 232)', 'rgb(114, 47, 55)'])
  assert.deepEqual(hoverColors, ['rgb(246, 241, 232)', 'rgb(91, 37, 44)'])
  await action.focus(); assert.equal(await action.evaluate(e => getComputedStyle(e).outlineColor), 'rgb(114, 47, 55)')
  note('Rendered contrast: all measured text pairs >=4.5:1; input/invalid/outlined boundaries >=3:1; primary default/hover and focus match the checked palette. Disabled copy is readable; no text opacity.')
  // Cold load with external origins blocked, saved game identity, font/image failure.
  const offline = await browser.newContext({ viewport: { width: 375, height: 900 } })
  const offlineRequests = [], offlineSockets = []
  await offline.route('**/*', route => { const url = new URL(route.request().url()); return url.origin === origin ? route.continue() : route.abort() })
  await offline.addInitScript(() => {
    if (!localStorage.getItem('auctionlab.lang')) localStorage.setItem('auctionlab.lang', 'vi')
    localStorage.setItem('auctionlab.player_token', 'old-game-token-preview')
  })
  offline.on('request', r => offlineRequests.push({ path: new URL(r.url()).pathname, origin: new URL(r.url()).origin, type: r.resourceType() }))
  const op = await offline.newPage(); op.on('websocket', ws => offlineSockets.push(ws.url()))
  await ready(op, origin + '/')
  assert.equal(await op.locator('html').getAttribute('lang'), 'vi')
  await op.locator('header a[href="/lots"]').click(); await op.waitForURL(origin + '/lots')
  assert.equal(await op.locator('html').getAttribute('lang'), 'vi')
  await ready(op, origin + '/account/watchlist')
  assert(!(await op.locator('body').innerText()).includes('old-game-token-preview'))
  assert.equal(await op.locator('html').getAttribute('lang'), 'vi')
  await ready(op, origin + '/lots')
  const fontCheck = await op.evaluate(async () => {
    const sample = 'Những câu chuyện được lưu giữ. Đấu giá hiện vật, khoá quản trị.'
    for(const family of ['Noto Sans','Noto Serif']) for(const weight of [400,600]) await document.fonts.load(`${weight} 16px "${family}"`, sample)
    return [...document.fonts].filter(f => f.status === 'loaded').map(f => ({ family:f.family, weight:f.weight,status:f.status }))
  })
  assert.equal(fontCheck.length, 8)
  await screenshot(op, 'offline-vietnamese-375.png')
  // Verify actual custom-font use, rather than only the CSS family declaration.
  const cdp = await offline.newCDPSession(op)
  await cdp.send('DOM.enable'); await cdp.send('CSS.enable')
  async function usedFonts(selector) {
    const document = await cdp.send('DOM.getDocument')
    const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: document.root.nodeId, selector })
    return (await cdp.send('CSS.getPlatformFontsForNode', { nodeId })).fonts
  }
  const renderedFonts = { heading: await usedFonts('h1'), body: await usedFonts('.lede') }
  assert(renderedFonts.heading.some(f => f.isCustomFont && f.familyName.includes('Noto Serif')))
  assert(renderedFonts.body.some(f => f.isCustomFont && f.familyName.includes('Noto Sans')))
  results.renderedFonts = renderedFonts
  // A single failed illustration gets the real local SVG placeholder and caption.
  await op.locator('.lot-image img').first().evaluate(e => { e.src = '/site-assets/forced-missing-image.svg' })
  await op.locator('.lot-image figcaption').first().waitFor()
  await op.waitForFunction(() => { const e=document.querySelector('.lot-image img');return e.src.endsWith('/site-assets/image-unavailable.svg') && e.naturalWidth>0 })
  await screenshot(op, 'local-image-fallback-375.png')
  await op.route('**/*.woff2', route => route.abort())
  await op.route('**/site-assets/image-unavailable.svg', route => route.abort())
  await op.route('**/*', route => route.request().resourceType() === 'image' ? route.abort() : route.fallback())
  await ready(op, origin + '/lots')
  // Inline SVG imports have no HTTP request; force their error path as a failed image decode.
  await op.locator('.lot-image img').evaluateAll(els => els.forEach(e => { e.src = '/site-assets/forced-missing-image.svg' }))
  await op.locator('.lot-image figcaption').first().waitFor()
  assert.equal(await op.locator('.lot-image figcaption').count(), 3)
  assert(await op.locator('h1').isVisible())
  assert.equal((await layoutCheck(op)).smallTargets.length, 0)
  const fallbackFonts = { heading: await usedFonts('h1'), body: await usedFonts('.lede') }
  assert(fallbackFonts.heading.every(f => !f.isCustomFont))
  assert(fallbackFonts.body.every(f => !f.isCustomFont))
  results.fallbackFonts = fallbackFonts
  assert.equal(await op.locator('.lot-card a').count(), 3)
  await screenshot(op, 'forced-font-image-failure-375.png')
  assert.equal(offlineSockets.length, 0)
  assert.equal(offlineRequests.filter(r => /^\/(api|ws|images)(\/|$)/.test(r.path)).length, 0)
  assert.equal(offlineRequests.filter(r => r.origin !== origin).length, 0)
  await offline.close()
  note('Cold browser with external origins blocked: all 8 local font subsets/weights load with Vietnamese text; local illustrations render; preference survives a real link. Forced font/image/fallback failures retain readable text/captions, card dimensions, and detail links. Saved game tokens cause no restore, game WebSocket, or business request.')
  // Verify cache/404 contracts over real production HTTP (also API boundary).
  const assets = await readdir('web/dist/assets')
  for (const path of ['/assets/missing.svg','/assets/missing.woff2','/site-assets/missing.svg','/site-assets/missing.woff2']) {
    const response = await fetch(origin+path,{headers:{Accept:'text/html'}})
    assert.equal(response.status,404); assert(!(await response.text()).includes('<html')); assert(!response.headers.get('cache-control')?.includes('immutable'))
  }
  for(const file of assets.filter(f => /\.(woff2|js|css)$/.test(f))) {
    const response = await fetch(origin+'/assets/'+file); assert.equal(response.status,200); assert.equal(response.headers.get('cache-control'),'public, max-age=31536000, immutable')
  }
  const fallback = await fetch(origin+'/site-assets/image-unavailable.svg'); assert.equal(fallback.status,200); assert(!fallback.headers.get('cache-control')?.includes('immutable'))
  const api = await fetch(origin+'/api/no-such-endpoint'); assert.equal(api.status,404); assert(api.headers.get('content-type').includes('application/json'))
  note('Embedded HTTP: documents no-cache; all built fonts/CSS/JS immutable; fallback SVG local and non-immutable; both namespaces return 404 without HTML or immutable caching; unknown API stays JSON 404.')
  assert.equal(sockets.length, 0); assert.equal(business.length, 0); assert.equal(external.length, 0); assert.equal(results.errors.length, 0)
  // Development: direct load/refresh for every route; same native history/new-tab behavior.
  const dc = await browser.newContext(); const dp = await dc.newPage()
  for(const [path,heading,layout] of routes) {
    await ready(dp,dev+path); assert((await dp.locator('h1').innerText()).startsWith(heading),path)
    await dp.reload(); await dp.locator('h1').waitFor()
    results.routes.push({origin:'Vite',path,heading:await dp.locator('h1').innerText(),layout,direct:'pass',refresh:'pass'})
  }
  await ready(dp,dev+'/'); await dp.locator('header a[href="/account"]').click(); await dp.waitForURL(dev+'/account')
  await dp.goBack(); await dp.waitForURL(dev+'/'); await dp.goForward(); await dp.waitForURL(dev+'/account')
  const dpopPromise=dc.waitForEvent('page'); await dp.locator('header a[href="/lots"]').click({button:'middle'})
  const dpop=await dpopPromise; await dpop.waitForLoadState();assert.equal(new URL(dpop.url()).pathname,'/lots');await dpop.close()
  for(const path of ['/assets/missing.woff2','/assets/missing.svg','/site-assets/missing.woff2','/site-assets/missing.svg']) {
    const response=await dc.request.get(dev+path,{headers:{Accept:'text/html'}});assert.equal(response.status(),404);assert(!(await response.text()).includes('<html'))
  }
  const publicAsset=await dc.request.get(dev+'/site-assets/image-unavailable.svg');assert.equal(publicAsset.status(),200)
  const devFont=await dc.request.get(dev+'/src/assets/fonts/noto-sans-latin-400-normal.woff2');assert.equal(devFont.status(),200)
  await dc.close()
  note('Vite: every route direct-loads/refreshes; Back/Forward/new-tab pass; admin is a public empty visual preview; real local fonts/fallback load and missing fonts/images in both reserved namespaces return 404 even with Accept:text/html.')
  await writeFile(join(output,'browser-results.json'),JSON.stringify(results,null,2)+'\n')
  console.log('ACCEPTANCE PASS', results.routes.length, 'route checks;', results.matrix.length, 'layout checks;', results.contrast.length, 'rendered pairs.')
} catch(error) {
  // No server logs or key-bearing URLs are written to evidence.
  console.error('ACCEPTANCE FAILURE:',String(error.message).replaceAll(key,'[redacted]'))
  if(browser) { for(const context of browser.contexts()) for(const page of context.pages()) if(!page.isClosed() && !page.url().includes('?k=')) await screenshot(page,'failure.png').catch(()=>{}) }
  process.exitCode=1
} finally {
  await browser?.close()
  const stopped=once(server,'exit'); server.kill('SIGTERM'); await stopped
}

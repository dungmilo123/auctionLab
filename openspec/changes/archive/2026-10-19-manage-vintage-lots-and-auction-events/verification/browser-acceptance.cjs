// Run with PLAYWRIGHT_MODULE pointing to an installed official Playwright package.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const fs = require('node:fs/promises')
const path = require('node:path')
const assert = require('node:assert/strict')
const output = __dirname
const origin = 'http://127.0.0.1:8080'
const key = process.env.ACCEPTANCE_ADMIN_KEY
if (!key) throw new Error('ACCEPTANCE_ADMIN_KEY required for isolated fixture only')
const checks = []
async function waitFor(page, text) { await page.getByText(text, { exact: true }).first().waitFor() }
async function overflow(page, label) {
  const result = await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth }))
  assert.ok(result.scroll <= result.width + 1, `${label}: horizontal overflow ${JSON.stringify(result)}`)
}
async function keyboardTo(page, predicate, maximum = 180) {
  for (let i = 0; i < maximum; i++) {
    await page.keyboard.press('Tab')
    const active = await page.evaluate(() => ({ text: document.activeElement?.textContent, label: document.activeElement?.getAttribute('aria-label'), id: document.activeElement?.id }))
    if (predicate(active)) return active
  }
  throw new Error('Keyboard target was not reachable')
}
;(async () => {
  const browser = await chromium.launch({ headless: true })
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
    const errors = []
    const page = await context.newPage()
    let sockets = 0
    page.on('websocket', () => sockets++)
    page.on('pageerror', err => errors.push(err.message))
    await context.request.get(`${origin}/admin?k=${encodeURIComponent(key)}`)
    await page.goto(`${origin}/admin/lots?edit=demo-lot-01`)
    await page.locator('#catalog-title').waitFor()
    const title = 'A carefully collected folding travel camera with a long operator-entered title'
    await page.locator('#catalog-title').fill(title)
    await page.locator('#catalog-starting_price_minor').fill('125.501')
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    await page.getByRole('alert').waitFor()
    assert.equal(await page.locator('#catalog-starting_price_minor').inputValue(), '125.501')
    assert.equal(await page.locator('#catalog-title').inputValue(), title)
    await page.getByRole('button', { name: 'Tiếng Việt', exact: true }).click()
    assert.equal(await page.locator('#catalog-title').inputValue(), title)
    assert.equal(await page.locator('html').getAttribute('lang'), 'vi')
    await page.getByRole('button', { name: 'English', exact: true }).click()
    await page.locator('#catalog-starting_price_minor').fill('125.50')
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    await waitFor(page, 'Saved to PostgreSQL.')
    await page.reload(); await page.locator('#catalog-title').waitFor()
    assert.equal(await page.locator('#catalog-starting_price_minor').inputValue(), '125.50')
    assert.equal(await page.locator('#catalog-title').inputValue(), title)
    checks.push('Invalid money retained; valid cents/title persisted; EN/VI switching preserved input')
    // A second editor wins first; the visible unsaved draft must survive its stale response.
    const latest = (await (await context.request.get(`${origin}/api/admin/catalog/lots/demo-lot-01`)).json()).data
    const { id, state, created_at, updated_at, demo, event_id, ...replacement } = latest
    const newer = await context.request.put(`${origin}/api/admin/catalog/lots/${id}`, { headers: { 'X-AuctionLab-Admin': '1' }, data: { ...replacement, title: title + ' / newer' } })
    assert.equal(newer.status(), 200)
    await page.locator('#catalog-title').fill('Unsaved stale text')
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    await waitFor(page, 'A newer version exists. Reload before saving again.')
    assert.equal(await page.locator('#catalog-title').inputValue(), 'Unsaved stale text')
    assert.equal(await page.getByRole('button', { name: 'Save draft', exact: true }).isDisabled(), true)
    await page.getByRole('button', { name: 'Reload committed state', exact: true }).click()
    await page.locator('#catalog-title').waitFor()
    assert.equal(await page.locator('#catalog-title').inputValue(), title + ' / newer')
    checks.push('Stale save retains input and blocks overwrite until explicit committed-state reload')
    await page.route('**/api/admin/catalog/lots/demo-lot-01', route => route.request().method() === 'PUT' ? route.abort() : route.continue())
    await page.locator('#catalog-title').fill('Uncertain local text')
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    await waitFor(page, 'The save outcome is uncertain. Reload committed state before any retry.')
    assert.equal(await page.locator('#catalog-title').inputValue(), 'Uncertain local text')
    assert.equal(await page.getByRole('button', { name: 'Save draft', exact: true }).isDisabled(), true)
    await page.unroute('**/api/admin/catalog/lots/demo-lot-01')
    await page.getByRole('button', { name: 'Reload committed state', exact: true }).click()
    await page.locator('#catalog-title').waitFor()
    checks.push('Aborted mutation cannot show false success and requires reload')
    // Test lost authorization while preserving the visible editor input.
    await context.clearCookies()
    await page.locator('#catalog-title').fill('Retained after authorization loss')
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    await waitFor(page, 'Admin access is required.')
    assert.equal(await page.locator('#catalog-title').inputValue(), 'Retained after authorization loss')
    await context.request.get(`${origin}/admin?k=${encodeURIComponent(key)}`)
    checks.push('Authorization loss retains unsaved inputs and shows Go gate recovery')
    // Keyboard-only order controls, starting at document focus.
    const beforeOrder = (await (await context.request.get(`${origin}/api/admin/catalog/events/demo-event-2`)).json()).data.lot_ids
    await page.goto(`${origin}/admin/auctions?edit=demo-event-2`)
    await page.locator('#catalog-title').waitFor()
    await keyboardTo(page, active => active.label?.startsWith('Move down:'))
    await page.keyboard.press('Enter')
    await keyboardTo(page, active => active.text === 'Save lot order')
    await page.keyboard.press('Enter')
    await waitFor(page, 'Saved to PostgreSQL.')
    const event = (await (await context.request.get(`${origin}/api/admin/catalog/events/demo-event-2`)).json()).data
    assert.equal(event.lot_ids[0], beforeOrder[1])
    checks.push('Keyboard Tab/Enter reordered and persisted event lots; server schedule retained')
    // Responsive/language matrix on both working draft editors.
    for (const lang of ['en', 'vi']) {
      await page.getByRole('button', { name: lang === 'en' ? 'English' : 'Tiếng Việt', exact: true }).click()
      for (const width of [320, 375, 768, 1280]) {
        await page.setViewportSize({ width, height: 900 })
        for (const [kind, selection] of [['lots', 'demo-lot-07'], ['auctions', 'demo-event-2']]) {
          await page.goto(`${origin}/admin/${kind}?edit=${selection}`)
          await page.locator('#catalog-title').waitFor()
          assert.equal(await page.locator('html').getAttribute('lang'), lang)
          await overflow(page, `${kind}/${lang}/${width}`)
          const missingLabels = await page.locator('main input:not([type=checkbox]),main textarea,main select').evaluateAll(elements => elements.filter(el => !el.id || !document.querySelector(`label[for="${el.id}"]`)).length)
          assert.equal(missingLabels, 0)
          await page.screenshot({ path: path.join(output, `${kind}-${lang}-${width}.png`), fullPage: true })
        }
      }
    }
    checks.push('Both editor pages: EN/VI × 320/375/768/1280; no page horizontal overflow; labeled fields')
    // Text-only 200% enlargement: double a snapshot of computed font sizes.
    await page.getByRole('button', { name: 'English', exact: true }).click()
    await page.setViewportSize({ width: 375, height: 900 })
    for (const [kind, selection] of [['lots', 'demo-lot-07'], ['auctions', 'demo-event-2']]) {
      await page.goto(`${origin}/admin/${kind}?edit=${selection}`); await page.locator('#catalog-title').waitFor()
      await page.evaluate(() => { const sizes = [...document.querySelectorAll('body *')].map(el => [el, parseFloat(getComputedStyle(el).fontSize)]); for (const [el, size] of sizes) el.style.fontSize = `${size * 2}px` })
      await overflow(page, `${kind}/200%-text`)
      await page.screenshot({ path: path.join(output, `${kind}-200-percent-text.png`), fullPage: true })
    }
    checks.push('Both editors: 200% computed-font enlargement at 375px; no page horizontal overflow (text resize simulation, not browser chrome zoom)')
    // Missing image and disconnected-network behavior while editing.
    await page.goto(`${origin}/admin/lots?edit=demo-lot-07`); await page.locator('#catalog-title').waitFor()
    await page.locator('#catalog-images-0-path').fill('/images/missing-acceptance.svg')
    await page.getByText('Image unavailable. Illustration placeholder shown.', { exact: true }).first().waitFor()
    await page.screenshot({ path: path.join(output, 'missing-image.png'), fullPage: true })
    await context.setOffline(true)
    await page.locator('#catalog-title').fill('Offline retained input')
    await page.getByRole('button', { name: 'Save draft', exact: true }).click()
    await waitFor(page, 'The save outcome is uncertain. Reload committed state before any retry.')
    assert.equal(await page.locator('#catalog-title').inputValue(), 'Offline retained input')
    await context.setOffline(false)
    checks.push('Missing local image uses accessible fallback; disconnected save retains input without false success')
    // Direct creation uses stored data and native query navigation.
    await page.goto(`${origin}/admin/lots?new=1`); await page.locator('#catalog-title').waitFor()
    await page.locator('#catalog-title').fill('Browser-created draft')
    await page.getByRole('button', { name: 'Create draft', exact: true }).click()
    await page.waitForURL(/\?edit=/); await page.locator('#catalog-title').waitFor()
    await page.reload(); await page.locator('#catalog-title').waitFor()
    assert.equal(await page.locator('#catalog-title').inputValue(), 'Browser-created draft')
    await page.goto(`${origin}/admin/auctions?new=1`); await page.locator('#catalog-title').waitFor()
    await page.locator('#catalog-title').fill('Browser-created event')
    await page.getByRole('button', { name: 'Create draft', exact: true }).click()
    await page.waitForURL(/\?edit=/); await page.locator('#catalog-title').waitFor()
    await page.reload(); await page.locator('#catalog-title').waitFor()
    assert.equal(await page.locator('#catalog-title').inputValue(), 'Browser-created event')
    checks.push('Title-only lot/event creation follows stored editor URL and survives direct refresh')
    // Other roadmap destinations remain previews and never join a game.
    for (const destination of ['/', '/lots', '/account', '/account/orders', '/admin/orders']) {
      await page.goto(origin + destination)
      await page.locator('.preview-notice').waitFor()
      assert.equal(await page.locator('.catalog-form').count(), 0)
    }
    for (const destination of ['/screen', '/screen/retired']) {
      await page.goto(origin + destination)
      await page.getByRole('heading', { name: /^404/ }).waitFor()
    }
    await page.goto(`${origin}/admin/lots?edit=no-such-record`)
    await waitFor(page, 'This record or selection was not found.')
    assert.equal(await page.locator('#catalog-title').count(), 0)
    assert.equal(sockets, 0)
    checks.push('Public/account/order previews and retired screen 404s preserved; unknown editor does not fabricate a record; zero game WebSocket connections')
    // Vite document authorization is independent of API authorization.
    const anonymous = await browser.newContext(); const anonymousPage = await anonymous.newPage()
    await anonymousPage.goto('http://127.0.0.1:5173/admin/lots?edit=demo-lot-07')
    await waitFor(anonymousPage, 'Admin access is required.')
    assert.equal(await anonymousPage.locator('#catalog-title').count(), 0)
    await anonymous.request.get(`${origin}/admin?k=${encodeURIComponent(key)}`)
    await anonymousPage.reload(); await anonymousPage.locator('#catalog-title').waitFor()
    checks.push('Same-hostname Vite access: anonymous direct editor has no draft; Go gate cookie enables real editor')
    await anonymous.route('**/api/admin/catalog/**', route => route.abort())
    await anonymousPage.reload()
    await waitFor(anonymousPage, 'The application server or database is unavailable.')
    assert.equal(await anonymousPage.locator('#catalog-title').count(), 0)
    checks.push('Unavailable development API shows recovery with no mutable sample fallback')
    await anonymous.close()
    assert.deepEqual(errors, [])
    await fs.writeFile(path.join(output, 'browser-results.json'), JSON.stringify({ browser: await browser.version(), checks, screenshots: '16 language/width images, two text-resize images, one missing-image image', pageErrors: errors }, null, 2) + '\n')
    console.log(JSON.stringify({ status: 'passed', checks: checks.length }))
    await context.close()
  } finally { await browser.close() }
})().catch(error => { console.error(error.message); process.exit(1) })

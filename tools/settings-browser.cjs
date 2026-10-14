const assert = require('node:assert/strict');
const { chromium } = require(process.env.SMOKE_PLAYWRIGHT || 'playwright');
(async () => {
  const [origin, recovery] = process.argv.slice(2);
  const browser = await chromium.launch({ headless: true, executablePath: process.env.SMOKE_BROWSER });
  try {
    const page = await browser.newPage(); const errors = [], remote = [], writes = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', request => { if (request.method() === 'POST') writes.push(request.url()); });
    await page.route('**/*', route => {
      if ([origin, recovery].includes(new URL(route.request().url()).origin)) return route.continue();
      remote.push(route.request().url()); return route.abort();
    });
    await page.goto(origin + '/dates');
    await page.waitForFunction(() => !document.querySelector('#save').disabled);
    await page.locator('#eventName').fill('Fixture birthday');
    await page.locator('#eventDate').fill('2000-10-14');
    await page.locator('.zzz-pill.is-confirm').click();
    await page.waitForFunction(() => document.querySelector('#status').textContent.startsWith('Saved.'));
    const saved = await page.evaluate(() => CompanionAPI.request('dates.list'));
    assert.equal(saved.events[0].name, 'Fixture birthday');
    // Simulate another editor saving first; the page keeps its old revision.
    await page.evaluate(data => CompanionAPI.request('dates.save', { body: data }), saved);
    await page.locator('#eventName').fill('Stale edit');
    await page.locator('#eventDate').fill('2001-10-14'); await page.locator('.zzz-pill.is-confirm').click();
    await page.waitForFunction(() => document.querySelector('#status').textContent.includes('changed in another window'));
    assert.equal(writes.filter(url => url.endsWith('/api/v1/dates')).length, 3);
    assert.equal((await page.evaluate(() => CompanionAPI.request('dates.list'))).events.length, 1);

    await page.goto(origin + '/holidays');
    await page.waitForFunction(() => !document.querySelector('#country').disabled && !document.querySelector('#save').disabled);
    await page.locator('#country').selectOption('', { force: true }); await page.locator('.zzz-pill.is-confirm').click();
    await page.waitForFunction(() => document.querySelector('#status').textContent === 'Public holidays are off.');
    assert.equal((await page.evaluate(() => CompanionAPI.request('holidays.settings.get'))).country, '');

    await page.goto(origin + '/games');
    await page.locator('#file').setInputFiles({ name: 'Fixture.sfc', mimeType: 'application/octet-stream', buffer: Buffer.from('SNES fixture') });
    await page.waitForFunction(() => document.querySelector('#status').textContent.startsWith('Added'));
    await page.getByRole('button', { name: 'Play on TV', exact: true }).click();
    await page.waitForFunction(() => document.querySelector('#status').textContent.includes('is on the TV'));
    const cart = await page.evaluate(async () => (await CompanionAPI.request('games.carts.list')).carts[0]);
    assert.equal((await page.evaluate(() => CompanionAPI.request('games.selection.get'))).id, cart.id);
    await page.getByRole('button', { name: 'Saves', exact: true }).click();
    await page.getByRole('button', { name: 'Import…', exact: true }).waitFor();
    page.on('dialog', dialog => dialog.accept());
    // File import follows the page's confirmation and backup semantics.
    await page.getByRole('button', { name: 'Import…', exact: true }).click();
    await page.locator('#importFile').setInputFiles({ name: 'save.srm', mimeType: 'application/octet-stream', buffer: Buffer.from('fixture save') });
    await page.waitForFunction(() => document.querySelector('#status').textContent === 'Save imported.');
    const bytes = await page.evaluate(async id => {
      const url = CompanionAPI.url('games.carts.file.get', { path: { id, file: 'save' }, query: { download: 1 } });
      return await (await fetch(url)).text();
    }, cart.id);
    assert.equal(bytes, 'fixture save');
    const empty = await page.evaluate(id => CompanionAPI.request('games.carts.action', { path: { id, action: 'shelf' }, query: { on: 0 } }), cart.id);
    assert.equal(empty, null);

    for (const base of [origin, recovery]) {
      await page.goto(base + '/settings');
      await page.locator('#port').fill('9001');
      if (base === origin) await page.locator('.zzz-pill.is-confirm').click();
      else await page.getByRole('button', { name: 'Save settings', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#status').textContent === 'Saved. Restart the companion.');
      assert.equal((await page.evaluate(() => CompanionAPI.request('settings.get'))).port, 9001);
      assert.equal(new URL(page.url()).origin, base, 'saved ports do not redirect the listening origin');
    }
    assert.deepEqual(errors, []); assert.deepEqual(remote, []);
    console.log('Browser pages passed: date revision conflict/no retry, holiday save, ROM/save uploads, download/204 and normal/recovery restart settings, all local.');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });

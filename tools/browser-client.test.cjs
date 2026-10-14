const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const reply = (data, status = 200) => ({ ok: status < 400, status, text: async () => JSON.stringify(data) });
function harness(fetch) {
  const events = {}, timers = new Map(); let id = 0;
  const context = vm.createContext({ fetch, AbortController, Error, addEventListener: (name, fn) => events[name] = fn,
    setTimeout: (fn, ms) => { timers.set(++id, { fn, ms }); return id; }, clearTimeout: key => timers.delete(key) });
  for (const file of ['operations_generated.js', 'browser-client.js']) vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/companion', file), 'utf8'), context);
  return { api: context.CompanionAPI, events, timers };
}
test('browser operations encode parameters and distinguish raw uploads, JSON and empty replies', async () => {
  const calls = []; const file = new Uint8Array([1, 2, 3]);
  const h = harness(async (url, options) => { calls.push({ url, options }); return url.includes('/do/remove') ? reply(null, 204) : reply({ ok: true }); });
  await h.api.request('settings.save', { body: { port: 9001 } });
  await h.api.request('games.carts.add', { query: { name: 'a &b.nes' }, body: file });
  assert.equal(await h.api.request('games.carts.action', { path: { id: 'x', action: 'remove' } }), null);
  assert.equal(calls[0].options.body, '{"port":9001}');
  assert.equal(calls[1].url, '/api/v1/carts?name=a%20%26b.nes');
  assert.equal(calls[1].options.body, file);
  assert.equal(calls[1].options.headers['Content-Type'], 'application/octet-stream');
  assert.equal(calls[2].options.body, undefined);
  assert.equal(h.api.url('games.carts.file.get', { path: { id: 'a b', file: 'save' }, query: { download: 1 } }), '/api/v1/carts/a%20b/save?download=1');
  assert.equal(h.timers.size, 0);
});
test('browser errors retain operation/status/payload and never retry writes', async () => {
  let calls = 0;
  const h = harness(async () => { calls++; return reply({ error: 'stale revision' }, 409); });
  await assert.rejects(h.api.request('dates.save', { body: { version: 1, revision: 0, events: [] } }), e => e.operationId === 'dates.save' && e.status === 409 && e.payload.error === 'stale revision' && e.category === 'api');
  assert.equal(calls, 1);
  const invalid = harness(async () => ({ ok: true, status: 200, text: async () => 'bad JSON' }));
  await assert.rejects(invalid.api.request('dates.list'), e => e.category === 'invalid-response');
  assert.throws(() => h.api.url('games.carts.file.get', { path: { id: '..', file: 'save' } }), /invalid path/);
  await assert.rejects(h.api.request('maps.tile', { path: { z: 1, x: 1, y: 1 } }), /native binary/);
});
test('page exit and deadline cancel requests without sending writes again', async () => {
  const h = harness((url, options) => new Promise((resolve, reject) => options.signal.addEventListener('abort', () => reject(Error('aborted')))));
  const request = h.api.request('dates.list'); h.events.pagehide();
  await assert.rejects(request, e => e.name === 'AbortError' && e.category === 'cancelled');
  const timed = h.api.request('settings.get'); [...h.timers.values()][0].fn();
  await assert.rejects(timed, e => e.category === 'timeout'); assert.equal(h.timers.size, 0);
});

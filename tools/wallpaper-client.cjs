// Real handler/client smoke test, launched by TestWallpaperClientIntegration.
const assert = require('node:assert/strict');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
(async () => {
  const origin = new URL(process.argv[2]);
  assert.equal(origin.hostname, '127.0.0.1');
  const file = path.join(process.argv[3], 'script/services/companion.js');
  const { CompanionClient, CompanionState } = await import(pathToFileURL(file).href);
  const client = new CompanionClient(Number(origin.port));
  const health = await client.getHealth();
  assert.equal(health.name, 'zzz-wallpaper-companion');
  assert.equal(health.capabilities.lyrics, true);
  const stats = await client.getSystemStats();
  assert.ok(stats.sampledAt > 0);
  // Invalid metadata exercises the existing handler without any provider request.
  await assert.rejects(client.getLyrics({ unexpected: true }), error => error.operationId === 'lyrics.lookup' && error.status === 400 && error.payload.error === 'invalid lyrics request' && error.category === 'api');
  await client.checkHealth();
  assert.equal(client.state, CompanionState.ONLINE);
  const dates = await client.getDates();
  assert.equal(dates.version, 1);
  assert.equal(dates.events.length, 0);
  const event = { id: 'integration', name: 'Saved date', date: '2026-10-14', kind: 'custom', yearly: false };
  const saved = await client.saveDates({ ...dates, events: [event] });
  assert.equal(saved.revision, dates.revision + 1);
  assert.deepEqual((await client.getDates()).events, [event]);
  await assert.rejects(client.saveDates(dates), error => error.operationId === 'dates.save' && error.status === 409 && error.category === 'api');
  assert.deepEqual((await client.getDates()).events, [event]);
  const holidays = await client.getHolidays(2026);
  assert.equal(holidays.year, 2026);
  assert.equal(holidays.country, '');
  assert.deepEqual(holidays.records, []);
  const { EventStore } = await import(pathToFileURL(path.join(process.argv[3], 'script/room/calendar/calendar_events.js')).href);
  const { DateSync } = await import(pathToFileURL(path.join(process.argv[3], 'script/room/calendar/date_sync.js')).href);
  const { HolidaySync } = await import(pathToFileURL(path.join(process.argv[3], 'script/room/calendar/holiday_sync.js')).href);
  const values = new Map();
  const storage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value) };
  const store = new EventStore(storage);
  store.upsert({ ...event, id: 'migration', name: 'Legacy date' });
  const sync = new DateSync(store, () => {}, client);
  const holidaySync = new HolidaySync(storage, () => {}, client);
  try {
    sync.configure(client.port); holidaySync.configure(client.port, true, 2026);
    const deadline = Date.now() + 5000;
    while ((sync.busy || holidaySync.busy) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 10));
    assert.equal(sync.migrationStatus, 'complete');
    assert.equal(store.events.length, 2);
    assert.equal((await client.getDates()).events.length, 2);
    assert.equal(holidaySync.selection.country, '');
  } finally { sync.dispose(); holidaySync.dispose(); }
  // Browser/media consumers use generated URLs directly, retaining tile caches.
  const tileURL = client.getMapTileUrl(2, 1, 1);
  assert.equal(client.getMapTileUrlTemplate().replace('{z}', '2').replace('{x}', '1').replace('{y}', '1'), tileURL);
  let tileBytes;
  for (let i = 0; i < 2; i++) {
    const tile = await fetch(tileURL);
    assert.equal(tile.status, 200);
    assert.equal(tile.headers.get('content-type'), 'image/png');
    assert.match(tile.headers.get('cache-control'), /^private, max-age=/);
    const bytes = Buffer.from(await tile.arrayBuffer());
    assert.equal(bytes.subarray(0, 8).toString('hex'), '89504e470d0a1a0a');
    if (tileBytes) assert.deepEqual(bytes, tileBytes); else tileBytes = bytes;
  }
  assert.equal((await fetch(client.getMapTileUrl(20, 1, 1))).status, 400);
  const { YouTubeSession } = await import(pathToFileURL(path.join(process.argv[3], 'script/services/companion/youtube.js')).href);
  const released = new YouTubeSession(client), retained = new YouTubeSession(client);
  await assert.rejects(released.resolve('bad!'), error => error.operationId === 'youtube.resolve' && error.status === 400);
  await assert.rejects(released.playlist('bad!'), error => error.operationId === 'youtube.playlist' && error.status === 400);
  await released.heartbeat(true); await retained.heartbeat(true); await client.checkHealth(); await released.release();
  assert.notEqual(released.id, retained.id); assert.notEqual(retained.id, client.sessionId);
  console.log('LEASE_IDS=' + JSON.stringify({ released: released.id, retained: retained.id, health: client.sessionId }));
  assert.equal(client.requests.size, 0);
  console.log('Generated wallpaper client passed against real Companion: health, heartbeat, system stats, lyrics validation, date read/save/conflict, calendar migration, holiday adapter, cached binary tiles and independent YouTube leases.');
})().catch(error => { console.error(error); process.exitCode = 1; });

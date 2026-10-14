# Companion API compatibility inventory

Date: 2026-10-14
Scope: first implementation slice; existing handlers and wallpaper transport retained.

## Source baseline

- Wallpaper: zzz_git, refactor/tv-scene-registry, c75f129b1b64a7d678c2ef1f1a269336eb3fd4c5.
- Companion implementation: existing checkout C:/Users/Dream/AppData/Local/Temp/zzz-companion-review, fix/game-station-stability-ui, 2bdc079.
- Sibling zzz-wallpaper-companion main is 0343e09 (v1.3); it lacks the ten game/input operations below. It is not the implementation target for this slice.
- These are observed source baselines, not release pins. Compatible release revision pins and generated client digests belong to later phases.
- Canonical contract: Companion api/openapi.json, embedded directly by api/contract.go. No duplicate generated contract copy.
- 29 API operations, 9 page/resource routes; GET also matches HEAD through Go ServeMux. OPTIONS is handled before route lookup.

## Common wire behavior

Server: IPv4 loopback, configurable port 1024–65535 (default 8765); saved port changes require restart. Header read timeout 5 s, request read timeout 15 s, idle timeout 60 s, no response write timeout. Local clients only. CORS accepts absent/null origin or HTTP localhost/127.0.0.1 with explicit port. Other clients/origins get 403 text/plain. Preflight returns 204, methods GET/POST/OPTIONS, header Content-Type. X-Cart-Rev is accepted by the save handler but is not in current CORS allowed headers: preserve/document this limitation in this slice.

JSON responses: application/json; charset=utf-8; JSON failures {error:string}. Middleware and unmatched route/method responses can be plain text. CORS sets Cache-Control:no-store; map responses override with private,max-age. No authentication tokens. Health readiness follows FFmpeg and is distinct from protocol compatibility. Current health capability map advertises youtubeRelay, lyrics, systemStats, mapTiles, importantDates; it does not yet advertise game/input capabilities.

Protocol and /v1 URL prefixes are independent. Heartbeats negotiate protocol 2–2. General connectivity and YouTube relay leases currently have separate session IDs/owners. This slice does not consolidate them.

## API operations

Detailed properties, query/path/header parameters, body limits, response content types and operation policies are in the canonical OpenAPI document. Status codes below are handler outcomes, in addition to common 403/mux errors.

| Method/path | Operation | Caller(s) | Outcomes and side effects |
| --- | --- | --- | --- |
| GET /api/health | health.get | shared transport fallback; weather capability probe; external diagnostics | 200 dependency readiness/capabilities; no write |
| POST /api/v1/heartbeat | heartbeat.renew | shared transport | 200 lease/compatibility; 400 invalid body/session; 426 mismatch; renew/release this session |
| POST /api/youtube/heartbeat | heartbeat.renewLegacy | YouTube adapter | Identical handler and wire body; deprecated alias remains usable |
| POST /api/v1/lyrics | lyrics.lookup | music lyrics via shared client | 200 ready/instrumental/not_found; 400 decoding; 502 service/metadata failure; memory/disk caches, prior uncached lookup cancellation |
| GET /api/v1/system-stats | system.stats | system channel via shared client | 200 latest sample; GPU omitted when unavailable |
| GET /api/v1/dates | dates.list | DateSync; dates page | 200 version/revision/events; 500 corrupt file preserved |
| POST /api/v1/dates | dates.save | DateSync legacy migration; dates page | 200 incremented revision; 400 validation/trailing JSON; 409 stale revision; 500 persistence/load; full document replacement |
| GET /api/v1/holiday-settings | holidays.settings.get | holidays page | 200 country/subdivision; 500 unreadable saved file |
| POST /api/v1/holiday-settings | holidays.settings.save | holidays page | 200 selection; 400 invalid/unknown selection; 503 unavailable discovery; 500 persistence/load; writes settings, may refresh countries |
| GET /api/v1/holiday-countries | holidays.countries | holidays page | 200 countries/stale/error even when discovery fails; TW/CN fallback; refresh/persist cache |
| GET /api/v1/holiday-subdivisions | holidays.subdivisions | holidays page | 200 array; 400 country/year; 503 provider failure; TW/CN empty array; refresh cache |
| GET /api/v1/holidays | holidays.list | HolidaySync | 200 calendar (may available=false); 400 year; 500 saved file; refresh cache |
| GET /api/v1/map-tiles/{z}/{x}/{y} | maps.tile | Leaflet/weather | 200 image/png; 400 coordinates; 500 cache failure; 502 provider/body failure; 503 backoff; disk cache |
| GET /api/youtube/resolve | youtube.resolve | YouTube | 200 media URLs; 400 id; 502 resolution; 503 installation, Retry-After:15; resolver cache |
| GET /api/youtube/playlist | youtube.playlist | external API; no current wallpaper caller found | 200 id/items; 400 id; 502 resolver |
| GET /api/youtube/hls | youtube.hls | proxy-rewritten media/manifest URLs | upstream statuses incl 206; 400 URL; 502 upstream/read; forwards Range, Accept-Ranges, Content-Range; manifest rewrite |
| GET /api/youtube/transcode | youtube.transcode | YouTube video element | 200 live video/webm; 400 id/session; 409 inactive lease; 503 FFmpeg unavailable; 502 resolve; 500 process/guard; replaces same-session relay |
| GET /api/settings | settings.get | external API; settings page is server-rendered | 200 saved settings |
| POST /api/settings | settings.save | settings page | 200 saved/restartRequired; 400 decode/validation/config persistence; 500 Windows startup; writes settings/startup |
| GET /api/v1/carts | games.carts.list | game deck; games page | 200 carts; GET creates ROM directory and migrates loose ROMs/saves; requires explicit explorer action |
| POST /api/v1/carts | games.carts.add | games page | raw ROM plus name query; 200 existing SHA1 match, 201 added; 400 file; 500 disk; creates cart/may migrate loose ROMs |
| GET /api/v1/carts/{id}/{file} | games.carts.file.get | retro ROM/save/state loading; games page downloads/label | 200/206 binary or PNG; 304 conditions; 416 range; 404 absent/invalid file; attachment when download=1 |
| POST /api/v1/carts/{id}/{file} | games.carts.file.save | retro periodic saves/state | raw bytes, optional X-Cart-Rev; 204 success/unchanged; 400 body; 404 writable file; 409 revision; 500 disk; backups and atomic writes |
| GET /api/v1/carts/{id}/details | games.carts.details | games page | 200 cart/save/saveBackup/state/states; nullable file details; 404 cart; persists missing ROM hash |
| POST /api/v1/carts/{id}/do/{action} | games.carts.action | games page | reset/import/restore:200 rev; remove/shelf:204; 400 import; 404 cart/action/backup; 409 shelf full; 500 disk; saves shelved or cart moved aside |
| GET /api/v1/games/current | games.selection.get | game deck watcher; games page | 200 id/seq/fresh; in-memory sequence resets on restart |
| POST /api/v1/games/current | games.selection.save | game shelf/deck; games page | 200 selection; 400 JSON; 404 cart; changes selection, may migrate loose ROMs |
| GET /api/v1/input | input.stream | retro EventSource | 200 SSE input snapshots, 15 s comment keep-alive; 500 no Flusher; starts native polling, last disconnect releases capture |
| POST /api/v1/input/play-mode | input.playMode.save | retro capture control | 200 input snapshot; 400 request/capture; changes keyboard/controller capture while subscriber exists |

The literal carts/{id}/details GET route wins over carts/{id}/{file}; equivalent/ambiguous patterns must be checked with ServeMux semantics, not raw strings.

## Caller timeouts, polling and ownership

- Shared client: default JSON timeout 5 s; heartbeat timeout 3 s every 5 s, stale after one failure/offline after two; fallback health only on heartbeat 404/405. Lyrics 15 s; system stats 3 s. Caller abort signals are linked to internal request controllers. Current port changes abort heartbeat only; later transport work must invalidate all old requests.
- DateSync: 4 s for read plus optional legacy merge write, 15 s polling. Retains last-good local data; migration marker keyed by port; revision conflicts prevent overwrites. Existing polling may attempt migration again after a failed save. The later migration must address uncertain write completion without introducing blind automatic retries.
- HolidaySync: 30 s per request, 60 s polling, keyed by port/country/subdivision/year. Retains last-good available data and ignores stale visit/generation responses. Disabled/disposed consumers stop polling.
- Weather: shared health probe uses default 5 s; Leaflet owns tile loading, cancellation and visible tile requests. No JSON timeout on image tiles; server provider timeout 15 s, four request slots. Provider no-store/no-cache/max-age/Age/Expires respected, fallback TTL seven days; no stale-on-error tile fallback. 403/429 upstream enters at least one-hour backoff.
- YouTube: heartbeat 3 s, image probe 2.5 s, periodic heartbeat scheduling owned by YouTube. Resolve fetch uses visit/controller cancellation with no explicit fetch timer; server resolution context 50 s. HLS upstream uses 2-minute HTTP client timeout. Relay has 20 s leases checked every second and no write timeout. Pause release delay 30 s; live media retry/URL refresh remains feature-owned.
- Game deck: selection polling 2 s; no explicit request timeout. Cart list and ROM/save loading use caller signal; writes have no explicit timeout. Retro save interval 30 s. Uncertain save response remains offline, no new retries in this slice. Save revision distinguishes externally replaced/reset saves; missing revision stays accepted for old clients. Prior battery save retained as .bak; three state backups rotate when prior state is at least ten minutes old.
- Input: EventSource reconnect and consumer release owned by retro adapter; failed stream retry 5 s. Capture modes desktop/key/always; empty capture means desktop. Input snapshot at is Unix milliseconds; buttons is RetroPad bitmask.
- Browser settings/date/holiday/game pages generally have no fetch-level timeout; server/provider policies above apply. Date and holiday JSON writers reject trailing data; other JSON handlers currently decode only the first object. Do not silently normalize this difference.
- Examples: lyrics cache fixture from internal/lyrics/lyrics_test.go; leap-day date/revision fixture from dates_test.go; US New Year record from holidays_test.go. No made-up provider response is used as captured evidence.

## Non-API page and resource routes

| Route | Content / behavior |
| --- | --- |
| GET / | 307 redirect /settings; catch-all checks exact root, other unmatched paths 404 |
| GET /settings | Server-rendered settings HTML; now links to /docs |
| GET /dates | Embedded dates editor HTML |
| GET /holidays | Embedded holiday settings HTML |
| GET /games | Embedded game manager HTML |
| GET /licenses/unicode | Unicode license text |
| GET /resource/svg/cross.svg | 1-pixel SVG, no-store; legacy image probe |
| GET /ui/theme.css | Theme plus pills CSS |
| GET /ui/shell.js | Build-info script plus shell/controls JS |

The port-conflict recovery listener serves only /settings and GET/POST /api/settings; no feature APIs or explorer. Its temporary port never silently replaces the wallpaper port.

New docs resources: GET /openapi.json, GET /docs, GET /docs/{asset}. Only enumerated bundled assets are served; unknown assets return 404. Swagger UI 5.33.1 is vendored from its official tag with license/notice and SHA256 manifest. No remote validator/CDN; origin and actual listening port determine execution URL. File/media/SSE calls cannot execute in Swagger; actions require a checkbox and per-request confirmation, including side-effecting GET cart listing.

## Slice verification and next gate

Go checks compare contract methods/paths against source route declarations, resolve schema references, ensure unique operation IDs and use ServeMux to check pattern conflicts. They check selected wire fields, bundled asset hashes, exact served contract, settings link and existing loopback/CORS restrictions. Browser checks must exercise the actual bundled UI on a nondefault port, reject remote requests and verify execution guards.

This is structural/selected wire coverage, not full runtime schema validation. Next: explicit operation registry and deterministic Go/client generation for health, lyrics and system statistics, followed by transport/cancellation work. Native streaming, external providers, real media playback and supported old/new release combinations remain later integration gates. No binary replacement, runtime sync or release is included here.

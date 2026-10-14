# Companion API contract

openapi.json is the canonical OpenAPI 3.0.3 document. api/contract.go embeds it directly; build and serving do not generate or download a second copy. The contract baseline includes games/input from fix/game-station-stability-ui (2bdc079). See ../docs/COMPANION-API-INVENTORY.md for callers and preservation requirements.

The normal Companion listener serves /openapi.json and /docs inside the existing local-only/CORS middleware. Port-conflict recovery serves settings only. Swagger UI uses the current listening origin even if saved settings already contain a different port awaiting restart.

## Checks

Run go test ./... for route coverage, unique operation IDs, schema reference resolution, actual ServeMux pattern checks, selected wire fields, documentation access policy and vendored asset integrity. Existing handlers keep semantic validation; these checks are not a runtime OpenAPI validator.

Browser integration is opt-in because Go builds have no Node/browser dependency:

PowerShell:

    $env:COMPANION_DOCS_BROWSER = '1'
    $env:SMOKE_PLAYWRIGHT = '<absolute path to playwright package>'
    $env:SMOKE_BROWSER = '<absolute browser executable path>'
    go test ./internal/companion -run TestDocsBrowser -count=1 -v

DOCS_NODE optionally selects Node; DOCS_SCREENSHOT optionally saves a screenshot. The test starts the real handler on a random loopback port with temporary settings/data, renders all 29 operations, runs health, blocks all external browser requests, and exercises the action/media guards. It does not launch the tray app or install media dependencies.

## Assets and updates

Swagger UI 5.33.1 assets are taken from https://github.com/swagger-api/swagger-ui/tree/v5.33.1. Runtime uses no CDN, remote validator or sample API. internal/apidocs/assets/vendor.json records exact source URLs and SHA256 hashes. Keep LICENSE and NOTICE with assets. Refresh deliberately from an official pinned tag, update version/hashes together, then run Go and browser checks. The embedded upstream bundle is unmodified.

Explorer GET execution is enabled initially. The action checkbox enables POST, and per-request confirmation also protects side-effecting GET cartridge listing. File/media/SSE endpoints always reject execution through the explorer. Reads may populate caches; this is recorded in each operation's side-effect metadata.

## Registry and generation

The normal listener binds all 29 operations by operation ID through internal/api. It uses the existing APIServer services and handlers. Pages and docs are separate resource groups checked on the same private ServeMux; duplicate/equivalent/ambiguous patterns and missing/unused handlers fail registration before the mux is exposed. More-specific routes and GET/HEAD semantics stay intact. Recovery settings routes remain independent.

Generate deliberately from this checkout (requires Node and gofmt):

    node tools/generate-contract.cjs --wallpaper "<wallpaper checkout>"
    node tools/generate-contract.cjs --check --wallpaper "<wallpaper checkout>"
    node --test tools/generate-contract.test.cjs

Omit --wallpaper to generate/check only server metadata. Generated outputs are committed for offline builds. Contract digests use SHA256 of UTF-8 JSON with CRLF normalized to LF. Generation does not edit the contract or implement feature behavior. Go tests compare embedded contract metadata/digest against generated bindings. Wallpaper npm test verifies its bundled artifact against contract-pin.json; the cross-repository --check compares it against the canonical source as well.

contract-pin.json records the Companion revision containing the canonical contract, its digest/version and the generated operation artifact digest. The source revision remains stable when the contract is unchanged. A changed canonical contract must be committed before generating wallpaper artifacts so the pin points to recoverable contract bytes. Release revision pairing for server/wallpaper builds remains a later release step.

The wallpaper facade now uses named health, heartbeat, lyrics, system, dates, holiday-calendar and YouTube operations. Calendar adapters receive the same app facade and retain their polling/cache lifecycle. Legacy date merging persists its one-time attempt before sending and reconciles through reads; uncertain outcomes never trigger a blind resend. Map tiles use generated concrete URLs/Leaflet templates; Leaflet retains binary loading/cache behavior. YouTubeSession uses generated resolve/playlist JSON (60-second budgets), legacy lease renewal and transcode/HLS URL builders. Renewals are serialized before release; each playback session captures its own port/ID, independently of shared health. Native media consumers retain streaming; JSON transport rejects binary/stream execution. Other SSE consumers still require dedicated adapters. Existing heartbeat protocol and stream ownership remain intact; playlist UI remains unsupported. There is no new client singleton or automatic write retry.

Real wallpaper-client integration is opt-in:

    $env:COMPANION_WALLPAPER_PATH = '<wallpaper checkout>'
    go test ./internal/companion -run TestWallpaperClientIntegration -count=1 -v

This starts the real handler on a random loopback port and exercises the generated client for health, heartbeat, system stats, lyrics validation, date read/save/conflict, both calendar adapters including legacy merge, and generated binary tile URLs against a local fixture provider. Tile checks include PNG signature bytes, private/max-age headers, invalid coordinates and cache reuse (two client reads, one provider download). YouTube checks exercise resolve/playlist validation and inspect server lease state after one playback session releases while another remains retained, independently of shared health. Test writes affect temporary data only. Native tray/media/Leaflet rendering, FFmpeg playback and live provider success are outside this test.

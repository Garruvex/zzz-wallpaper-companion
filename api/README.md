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

Next slice: explicit handler registry and deterministic generated operations/bindings for health, lyrics and system statistics. No client singleton, runtime consumer, protocol or streaming policy changes in this slice.

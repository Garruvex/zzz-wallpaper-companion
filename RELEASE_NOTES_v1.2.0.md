# ZZZ Wallpaper Companion v1.2.0

This backward-compatible release keeps protocol v2 and adds two optional APIs for future wallpaper inputs.

## New APIs

- `POST /api/v1/lyrics` accepts track metadata and returns the complete synchronized or plain lyric result.
- `GET /api/v1/system-stats` returns the latest cached CPU, GPU, and memory sample.
- `/api/health` now advertises `lyrics` and `systemStats` capabilities.

## Lyrics

- Cached results return immediately.
- Online lookup waits two seconds to avoid requests while users skip tracks.
- A newer track cancels the previous lookup.
- LRCLIB is tried first, with NetEase as a fallback.
- Successful results are cached in the companion data directory.

## System statistics

- A single background goroutine samples once per second.
- CPU and memory use native Windows APIs.
- GPU uses one persistent Windows PDH query; no PowerShell or polling subprocesses are launched.
- HTTP requests only serialize the latest in-memory snapshot.

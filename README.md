# ZZZ Wallpaper Companion

A small Windows background app for the [Zenless Zone Zero TV wallpaper](https://steamcommunity.com/sharedfiles/filedetails/?id=3333357727) on Wallpaper Engine. It runs in the notification area and lets the wallpaper play YouTube videos, live streams, and playlists.

**You need this companion for YouTube playback.** Without it, the wallpaper shows a message asking you to start it.

The companion does not bundle [yt-dlp](https://github.com/yt-dlp/yt-dlp) or FFmpeg. On first launch it downloads both tools for your CPU and verifies their published SHA-256 checksums. yt-dlp is kept updated automatically. FFmpeg is used only when a live stream must be converted from H.264/AAC HLS to Wallpaper Engine-compatible VP8/Opus WebM.

## Quick start

1. **Get the wallpaper** from the [Steam Workshop](https://steamcommunity.com/sharedfiles/filedetails/?id=3333357727) and apply it in Wallpaper Engine.

2. **Download** the [latest release](https://github.com/Garruvex/zzz-wallpaper-companion/releases/latest) for your PC (`amd64` for most PCs, `arm64` for ARM Windows such as Surface).

3. **Run** the executable. No installer — put it anywhere you like.

4. **Use the wallpaper.** The companion starts in the notification area (system tray). YouTube should work once it is running.

On first launch the companion downloads yt-dlp in the background. Give it a moment if playback does not work immediately.

## Requirements

- Windows 10 or later (`amd64` or `arm64`)
- Internet access (initial yt-dlp download and YouTube resolution)

## Tray menu

Right-click the notification-area icon. Double-click opens settings.

| Item | What it does |
| --- | --- |
| **Settings** | Open the settings page in your browser |
| **Launch on Windows startup** | Toggle automatic launch after sign-in |
| **Check for yt-dlp updates** | Update yt-dlp now |
| **Open data folder** | Open the companion data folder in Explorer |
| **Quit** | Stop the companion |

## Settings

Open **Settings** from the tray menu, or go to `http://127.0.0.1:8765/settings`.

| Setting | Default | Notes |
| --- | --- | --- |
| Companion port | `8765` | Must match what the wallpaper expects. Restart after changing. |
| Maximum resolution | `720` | `360`, `480`, `720`, or `1080` |
| Live-stream resolution | `360` | `240`, `360`, `480`, or `720`; higher values use substantially more CPU |
| yt-dlp channel | `nightly` | `nightly` (recommended) or `stable` |
| Launch on Windows startup | Off | Starts quietly in the notification area after sign-in |

## Troubleshooting

**Wallpaper says "Start the optional localhost companion"**

- Make sure `zzz-wallpaper-companion.exe` is running (check the notification area).
- Confirm the port in settings is `8765` (the wallpaper default).

**YouTube still does not play**

- Wait a minute on first run while yt-dlp downloads.
- Open **Open data folder** from the tray menu and check `companion.log` for errors.
- Try **Check for yt-dlp updates** from the tray menu.

**Port already in use**

- Change the companion port in settings, then restart the companion.
- The wallpaper must use the same port (default `8765`).

**Clean reset**

- Quit the companion, delete `%LOCALAPPDATA%\ZZZWallpaperCompanion`, and run again. Settings and yt-dlp will be re-downloaded.

## Data folder

```
%LOCALAPPDATA%\ZZZWallpaperCompanion\
```

| File | Purpose |
| --- | --- |
| `settings.json` | Port, resolution cap, yt-dlp channel |
| `yt-dlp.exe` | Downloaded resolver |
| `ffmpeg.exe` | Verified live-stream transcoder downloaded on first launch |
| `companion.log` | Runtime log |

Deleting the executable does not remove this folder.

---

## Developer reference

### API

The server binds to `127.0.0.1` only. Non-loopback requests are rejected. Browser `Origin` headers must be `localhost` or `127.0.0.1`.

#### `GET /api/health`

```json
{
  "ready": true,
  "name": "zzz-wallpaper-companion",
  "protocolVersion": 2,
  "version": "1.1.0",
  "protocolMin": 2,
  "protocolMax": 2,
  "ytDlpReady": true,
  "ffmpegReady": true
}
```

#### `GET /api/youtube/resolve?id=VIDEO_ID`

Resolves a video or live stream.

```json
{
  "videoUrl": "https://...",
  "audioUrl": "https://...",
  "videoFormatId": "248",
  "audioFormatId": "251",
  "extension": "webm",
  "width": 1280,
  "height": 720,
  "videoCodec": "vp9",
  "audioCodec": "opus",
  "title": "Example",
  "isLive": false,
  "muxed": false,
  "expiresAt": 1754512345
}
```

`audioUrl` is omitted when `muxed` is `true`. `expiresAt` is a Unix timestamp when available.

#### `GET /api/youtube/playlist?id=PLAYLIST_ID`

Returns up to 500 playlist entries (IDs only).

```json
{
  "id": "PLxxxxxxxx",
  "items": [
    { "id": "dQw4w9WgXcQ", "title": "Example", "duration": 212 }
  ]
}
```

#### `GET /api/settings` · `POST /api/settings`

Read or update settings. POST body:

```json
{
  "port": 8765,
  "maxHeight": 720,
  "updateChannel": "nightly",
  "launchOnStartup": false,
  "transcodeHeight": 360,
  "autoUpdate": true
}
```

Response includes `restartRequired: true` when the port changes.

Live transcoding supports 240p, 360p, 480p, 720p, and experimental 1080p. The
companion scales its WebM bitrate with the selected resolution; 1080p targets
4 Mbps and can require substantially more CPU than the lower settings.

Other routes: `GET /settings` (web UI), `GET /` (redirects to settings).

#### `POST /api/youtube/heartbeat`

Creates or refreshes a per-wallpaper stream lease. The wallpaper sends a
random `sessionId` every five seconds. Transcoded streams include that ID and
are cancelled if the heartbeat stops for 20 seconds.

```json
{ "sessionId": "random-session-id", "keepStreamAlive": true, "protocolMin": 2, "protocolMax": 2, "wallpaperVersion": "1.2.2" }
```

The companion rejects non-overlapping protocol ranges with HTTP 426 and tells
the wallpaper which component needs to be updated.

### Companion updates

The companion checks the latest stable GitHub release once per day when
automatic updates are enabled. It only stages releases whose protocol range
overlaps the running companion, downloads the build for the current Windows
architecture, and verifies its SHA-256 checksum from the release manifest.
The staged build installs on the next companion restart, keeps one rollback
executable, and must confirm that its localhost server started successfully or
the helper restores the previous build. The tray menu also provides a manual
update check.

### Build from source

Requires Go 1.23+.

The recommended Windows build command reads the version from `main.go`, runs
the tests once, embeds a UTC timestamp build number, and creates versioned
amd64 and arm64 GUI executables in `dist`:

```powershell
.\build.ps1
```

The build number and architecture can be supplied explicitly for a reproducible
release build:

```powershell
.\build.ps1 -BuildNumber 42 -Architecture amd64
```

```powershell
go test ./...
go build -ldflags "-H=windowsgui" -o dist\zzz-wallpaper-companion.exe .
```

Run `go generate` before building to embed the icon from `winres/icon.ico` into the executable (generates `rsrc_windows_*.syso`). CI does this automatically.

For development (console output), omit `-H=windowsgui`:

```powershell
go build -o dist\zzz-wallpaper-companion.exe .
```

Set `ZZZ_COMPANION_DATA_DIR` to use an isolated data directory during development.

GitHub Actions runs tests and builds `amd64` and `arm64` binaries on every push and pull request. Publish a [GitHub release](https://github.com/Garruvex/zzz-wallpaper-companion/releases) to make them available for download.

### Security

- HTTP API is loopback-only.
- CORS allows `http://127.0.0.1:*` and `http://localhost:*` only.
- yt-dlp is downloaded over HTTPS and verified against the upstream SHA-256 checksum.

## License

Companion source is [MIT](LICENSE). yt-dlp is downloaded separately from its upstream project and remains subject to its own licenses.

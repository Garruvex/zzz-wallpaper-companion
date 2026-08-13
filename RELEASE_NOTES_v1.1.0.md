# ZZZ Wallpaper Companion v1.1.0

Update for the optional companion for the [Zenless Zone Zero TV wallpaper](https://steamcommunity.com/sharedfiles/filedetails/?id=3333357727) on Wallpaper Engine.

This release adds **YouTube live stream playback**, bumps the companion API to **protocol v2**, and introduces **automatic companion updates** with verified downloads and rollback support. On-demand videos and playlists from v1.0 continue to work as before.

**Requires wallpaper v1.2.x** (protocol v2) for live streams. Update both the wallpaper and companion together.

---

## Download

Pick the build that matches your PC:

| | |
|---|---|
| **Most Windows PCs** | [zzz-wallpaper-companion-1.1.0-build-20260809085657-windows-amd64.exe](https://github.com/Garruvex/zzz-wallpaper-companion/releases/download/v1.1.0/zzz-wallpaper-companion-1.1.0-build-20260809085657-windows-amd64.exe) |
| **ARM Windows** (Surface, etc.) | [zzz-wallpaper-companion-1.1.0-build-20260809085657-windows-arm64.exe](https://github.com/Garruvex/zzz-wallpaper-companion/releases/download/v1.1.0/zzz-wallpaper-companion-1.1.0-build-20260809085657-windows-arm64.exe) |

No installer. Download the executable, run it, and leave it in the notification area while you use the wallpaper.

---

## Quick start

1. Update the [ZZZ TV wallpaper](https://steamcommunity.com/sharedfiles/filedetails/?id=3333357727) to v1.2.x in Wallpaper Engine.
2. Download and run the v1.1.0 companion executable for your CPU architecture.
3. Look for the icon in the Windows notification area (system tray).
4. Use the wallpaper — on-demand videos and live streams should work once the companion is running.

**First launch after upgrading:** the companion downloads [FFmpeg](https://github.com/BtbN/FFmpeg-Builds) in the background (in addition to yt-dlp). If live streams do not work right away, wait a minute and try again.

---

## What's new in v1.1.0

### YouTube live streams
Live broadcasts are transcoded from HLS (H.264/AAC) to VP8/Opus WebM so Wallpaper Engine can play them. FFmpeg is downloaded automatically on first use and verified against upstream SHA-256 checksums.

### Protocol v2 heartbeat
The wallpaper sends a session heartbeat every five seconds. Transcoded streams are cancelled automatically if the heartbeat stops (for example, when you switch away from YouTube on the TV).

### Automatic companion updates
The companion checks the latest stable GitHub release once per day when automatic updates are enabled (on by default). It only stages releases whose protocol range overlaps the running companion, downloads the build for your Windows architecture, and verifies its SHA-256 checksum from the release manifest. The staged build installs on the next companion restart. If the updated build does not confirm that its localhost server started successfully, the previous executable is restored automatically.

### Launch on Windows startup
New tray menu item and settings toggle to start the companion quietly in the notification area after sign-in.

### Live-stream resolution setting
Choose the transcoding resolution in settings: `240`, `360` (default), `480`, `720`, or experimental `1080`. Higher values use substantially more CPU.

### Reliability improvements
- Only one companion instance can run at a time; launching again shows a friendly message instead of starting a second copy.
- A startup notification balloon confirms the companion is running in the notification area.
- FFmpeg and yt-dlp child processes are cleaned up when the companion exits.

### Release build script
`build.ps1` reads the version from `main.go`, runs tests, and produces versioned `amd64` and `arm64` executables in `dist/`.

---

## Tray menu

Right-click the notification-area icon (double-click opens settings):

- **Version** — shows the running companion version and build number
- **Settings** — open the settings page in your browser
- **Launch on Windows startup** — toggle automatic launch after sign-in
- **Automatically download compatible updates** — toggle daily companion update checks
- **Check for companion updates** — check for a new companion release now
- **Check for yt-dlp updates** — update the resolver immediately
- **Open data folder** — open the companion data folder in Explorer
- **Quit** — stop the companion

---

## Settings

Open from the tray menu or visit `http://127.0.0.1:8765/settings`:

| Setting | Default | Description |
|---|---|---|
| Companion port | `8765` | Must match the wallpaper. Restart after changing. |
| Maximum resolution | `720` | `360`, `480`, `720`, or `1080` |
| Live-stream resolution | `360` | `240`, `360`, `480`, `720`, or `1080` |
| yt-dlp channel | `nightly` | `nightly` (recommended) or `stable` |
| Launch on Windows startup | Off | Starts quietly in the notification area after sign-in |
| Automatic companion updates | On | Download compatible verified releases and install on next restart |

---

## Data folder

All companion data lives in:

```
%LOCALAPPDATA%\ZZZWallpaperCompanion\
```

| File | Purpose |
|---|---|
| `settings.json` | Port, resolution cap, live-stream resolution, yt-dlp channel, startup toggle, auto-update toggle |
| `yt-dlp.exe` | Downloaded resolver binary |
| `ffmpeg.exe` | Downloaded live-stream transcoder (new in v1.1.0) |
| `companion.log` | Runtime log |
| `companion-update.exe` | Staged companion update (temporary, installed on next restart) |
| `companion-update.json` | Metadata for the staged update |
| `zzz-wallpaper-companion.exe.previous` | Rollback copy kept during companion self-updates |

Existing `settings.json` files from v1.0 are migrated automatically. Missing `transcodeHeight` defaults to `360`. Missing `autoUpdate` defaults to `true`.

---

## Upgrade notes (v1.0 → v1.1.0)

1. Update the wallpaper to v1.2.x first (or at the same time).
2. Replace the companion executable and run it.
3. Wait for FFmpeg to download on first launch (check `companion.log` in the data folder).
4. Live streams play at 360p by default — raise **Live-stream resolution** only if you have CPU headroom.

If automatic updates are enabled, future compatible companion releases can install themselves after you restart the app from the tray menu.

---

## Troubleshooting

**Wallpaper says "Start the optional localhost companion"**
- Confirm the companion is running (check the notification area).
- Verify the port in settings is `8765` (the wallpaper default).

**On-demand YouTube still does not play**
- Wait on first run while yt-dlp downloads.
- Open the data folder from the tray and check `companion.log`.
- Try **Check for yt-dlp updates** from the tray menu.

**Live streams do not play**
- Confirm the wallpaper is updated to v1.2.x.
- Wait for FFmpeg to finish downloading on first launch.
- Check `companion.log` for FFmpeg or transcoding errors.
- Try a lower **Live-stream resolution** in settings.

**Companion says it is already running**
- Look for the icon in the notification area — only one instance is allowed.
- Quit the existing companion from the tray menu before launching again.

**Companion update failed**
- Check `companion.log` for download or checksum errors.
- Try **Check for companion updates** from the tray menu.
- If rollback occurred, the previous build was restored automatically.

**Port already in use**
- Change the companion port in settings and restart.
- The wallpaper must use the same port.

---

## Full documentation

API reference, build instructions, and more: [README](https://github.com/Garruvex/zzz-wallpaper-companion/blob/v1.1.0/README.md)

**License:** Companion source is [MIT](https://github.com/Garruvex/zzz-wallpaper-companion/blob/v1.1.0/LICENSE). yt-dlp and FFmpeg are downloaded separately and remain subject to their own licenses.

# ZZZ Wallpaper Companion v1.0.0

The first release of the optional companion for the [Zenless Zone Zero TV wallpaper](https://steamcommunity.com/sharedfiles/filedetails/?id=3333357727) on Wallpaper Engine.

Without this app, the wallpaper cannot play YouTube content and will ask you to start the companion. With it running in the background, you can watch videos, live streams, and playlists directly on the in-game TV.

---

## Download

Pick the build that matches your PC:

| | |
|---|---|
| **Most Windows PCs** | [zzz-wallpaper-companion-windows-amd64.exe](https://github.com/Garruvex/zzz-wallpaper-companion/releases/download/v1.0.0/zzz-wallpaper-companion-windows-amd64.exe) |
| **ARM Windows** (Surface, etc.) | [zzz-wallpaper-companion-windows-arm64.exe](https://github.com/Garruvex/zzz-wallpaper-companion/releases/download/v1.0.0/zzz-wallpaper-companion-windows-arm64.exe) |

No installer. Download the executable, run it, and leave it in the notification area while you use the wallpaper.

---

## Quick start

1. Install the [ZZZ TV wallpaper](https://steamcommunity.com/sharedfiles/filedetails/?id=3333357727) from Steam Workshop and apply it in Wallpaper Engine.
2. Download and run the companion executable for your CPU architecture.
3. Look for the icon in the Windows notification area (system tray).
4. Use the wallpaper — YouTube playback should work once the companion is running.

**First launch:** the companion downloads [yt-dlp](https://github.com/yt-dlp/yt-dlp) automatically in the background. If playback does not work right away, wait a minute and try again.

---

## Highlights

### YouTube in the wallpaper
Resolves individual videos, live streams, and playlists through a small local HTTP API that the wallpaper talks to on `127.0.0.1`.

### Runs quietly in the background
A native Windows tray app — no console window, no installer, no admin rights required.

### Tray menu
Right-click the notification-area icon (double-click opens settings):

- **Settings** — open the settings page in your browser
- **Check for yt-dlp updates** — update the resolver immediately
- **Open data folder** — open the companion data folder in Explorer
- **Quit** — stop the companion

### Automatic yt-dlp management
The companion does not bundle yt-dlp. On first launch it downloads the official Windows binary for your CPU, verifies the published SHA-256 checksum, and keeps it updated (daily check, plus manual update from the tray).

### Settings
Open from the tray menu or visit `http://127.0.0.1:8765/settings`:

| Setting | Default | Description |
|---|---|---|
| Companion port | `8765` | Must match the wallpaper. Restart after changing. |
| Maximum resolution | `720` | `360`, `480`, `720`, or `1080` |
| yt-dlp channel | `nightly` | `nightly` (recommended) or `stable` |

### Built for security
- HTTP API binds to loopback only (`127.0.0.1`)
- Browser CORS allows `localhost` and `127.0.0.1` origins only
- yt-dlp downloads use HTTPS and are verified against upstream SHA-256 checksums

---

## Requirements

- Windows 10 or later (`amd64` or `arm64`)
- Internet access (initial yt-dlp download and ongoing YouTube resolution)

---

## Data folder

All companion data lives in:

```
%LOCALAPPDATA%\ZZZWallpaperCompanion\
```

| File | Purpose |
|---|---|
| `settings.json` | Port, resolution cap, yt-dlp channel |
| `yt-dlp.exe` | Downloaded resolver binary |
| `companion.log` | Runtime log |

Deleting the executable does **not** remove this folder. To reset everything, quit the companion, delete the folder, and run again.

---

## Troubleshooting

**Wallpaper says "Start the optional localhost companion"**
- Confirm the companion is running (check the notification area).
- Verify the port in settings is `8765` (the wallpaper default).

**YouTube still does not play**
- Wait on first run while yt-dlp downloads.
- Open the data folder from the tray and check `companion.log`.
- Try **Check for yt-dlp updates** from the tray menu.

**Port already in use**
- Change the companion port in settings and restart.
- The wallpaper must use the same port.

---

## Full documentation

API reference, build instructions, and more: [README](https://github.com/Garruvex/zzz-wallpaper-companion/blob/v1.0.0/README.md)

**License:** Companion source is [MIT](https://github.com/Garruvex/zzz-wallpaper-companion/blob/v1.0.0/LICENSE). yt-dlp is downloaded separately and remains subject to its own licenses.

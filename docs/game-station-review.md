# Game Station review — October 14, 2026

Reviewed merged PR #10 (1f658d32eba4a740d42591222804ec15fab6d216).

## Findings and changes

- Input reconnection could overlap two hub polling loops. The old loop can stop the singleton keyboard hook after the replacement loop starts it. Replacement loops now wait for the prior loop's keyboard shutdown. A regression test deliberately blocks old-hook shutdown during reconnection.
- Hook Stop published its stopped state before its thread had exited. Start/Stop now serialize the complete lifecycle, and hook handles belong to their own message-pump thread. The message queue is explicitly created before publishing readiness, following https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-postthreadmessagew . Pause state is cleared after shutdown.
- Windows GUI executables have no visible stderr; the observed gba-test12 restarts have no exception trace in companion.log. Runtime fatal errors and panics now also go to crash.log in the companion data folder. The user's exact crash has not been reproduced; these changes fix verified lifecycle defects, not a proven crash cause.
- Settings rendered the channel as a double-quoted string inside a JavaScript string. The dropdown was blank and saving failed validation. Use html/template's JavaScript escaping directly and explicitly bind form controls (including status, which otherwise clashes with window.status). Verified saving in the browser.

## UI

Shared embedded styles and navigation for Settings, Game Station, Important Dates, and Public Holidays. Black diagonal textures, yellow active tabs, heavy italic typography, rounded graphite rows and inset controls reflect the supplied game-settings reference. Responsive layouts and visible keyboard focus retain normal browser controls. No external fonts or assets are required.

## Validation

- go test ./... passes on Windows.
- go vet ./... passes.
- Input tests repeated ten times; Windows hook immediate stop/restart test runs 30 cycles per test invocation, with capture disabled.
- amd64 and arm64 executables build successfully; arm64 runtime behavior was not tested.
- Browser: settings Save reports Saved; dummy cartridge selects and opens Saves; all four page layouts checked, including narrow and desktop viewports.
- Running companion, settings, game files, and saves were not replaced or edited; preview used a separate server with temporary dummy data.

## Next runtime check

Run the review executable after exiting the existing companion from its tray menu. If it crashes, inspect C:\Users\Dream\AppData\Local\ZZZWallpaperCompanion\crash.log alongside companion.log, and record the action that triggered it. This build keeps the existing 1.3.0 version string and has a distinct review build ID; it is not a published release.

## UI revision 4

- Custom comboboxes replace native option popups across General, Holidays, and Important Dates. Checkbox settings use Disable/Enable menus. Selection rings and selected option fills breathe between neon yellow and green; reduced-motion preferences keep them static. Mouse, arrow keys, Enter, Escape, and Tab are supported. Native fields remain the source for validation and saving; dynamically loaded options and disabled states stay synchronized.
- Save and Reset reuse the approved pill geometry, colours, cap, and icons from zzz_git/style/ui/zzz_ui.css and docs/ui-templates/confirm-cancel-buttons.html. General Reset loads backend-provided defaults into the form and requires Save to persist; it does not reset game saves or dates. Save is guarded against duplicate submissions.
- Game library uses compact rows with aligned Playback, Shelf, Saves, and Library actions. Save details expand below a row. Focused game controls are not rebuilt by the periodic refresh.
- Slider CSS is prepared as .zzz-range with a value chip, broad track, and shaded circular thumb. No companion setting currently needs a continuous slider, so none was added to the live form.
- Browser verification: keyboard toggle selection, click selection, Save persistence after reload, Reset remaining staged after reload, dependent date-type/yearly toggle sync, game selection and save expansion, desktop and narrow layouts. Tests and vet pass; amd64 and arm64 build.

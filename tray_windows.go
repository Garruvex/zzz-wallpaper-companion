//go:build windows

package main

import (
	"context"
	_ "embed"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

//go:embed winres/icon.ico
var trayIconICO []byte

const (
	wmDestroy       = 0x0002
	wmCommand       = 0x0111
	wmApp           = 0x8000
	wmTray          = wmApp + 1
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203

	nimAdd     = 0x00000000
	nimModify  = 0x00000001
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010
	niifInfo   = 0x00000001

	idiApplication = 32512
	imageIcon      = 1
	lrDefaultSize  = 0x0040
	mfString       = 0x00000000
	mfChecked      = 0x00000008
	mfGrayed       = 0x00000001
	mfDisabled     = 0x00000002
	tpmRightButton = 0x0002
	cwUseDefault   = 0x80000000
	cmdSettings    = 1001
	cmdUpdate      = 1002
	cmdLogs        = 1003
	cmdQuit        = 1004
	cmdStartup     = 1005
	cmdAutoUpdate  = 1006
	cmdAppUpdate   = 1007
)

type point struct{ x, y int32 }
type msg struct {
	hwnd           uintptr
	message        uint32
	wParam, lParam uintptr
	time           uint32
	pt             point
}
type wndClassEx struct {
	size                               uint32
	style                              uint32
	wndProc                            uintptr
	clsExtra, wndExtra                 int32
	instance, icon, cursor, background uintptr
	menuName, className                *uint16
	iconSmall                          uintptr
}
type notifyIconData struct {
	size             uint32
	hwnd             uintptr
	id               uint32
	flags            uint32
	callbackMessage  uint32
	icon             uintptr
	tip              [128]uint16
	state, stateMask uint32
	info             [256]uint16
	timeoutOrVersion uint32
	infoTitle        [64]uint16
	infoFlags        uint32
	guid             [16]byte
	balloonIcon      uintptr
}

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	shell32             = syscall.NewLazyDLL("shell32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
	trayPort            int
	trayDataDir         string
	trayResolver        *Resolver
	trayUpdater         *UpdateManager
	trayConfig          *ConfigStore
	trayQuit            func()
)

func runTray(config *ConfigStore, dataDir string, resolver *Resolver, updater *UpdateManager, quit func()) error {
	trayPort, trayDataDir, trayResolver, trayUpdater, trayConfig, trayQuit = config.Get().Port, dataDir, resolver, updater, config, quit
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	className, _ := syscall.UTF16PtrFromString("ZZZWallpaperCompanionTray")
	wndProc := syscall.NewCallback(windowProc)
	wc := wndClassEx{size: uint32(unsafe.Sizeof(wndClassEx{})), wndProc: wndProc, instance: instance, className: className}
	atom, _, registerErr := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return registerErr
	}
	hwnd, _, createErr := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, cwUseDefault, cwUseDefault, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return createErr
	}
	icon := loadTrayIcon(instance)
	data := notifyIconData{size: uint32(unsafe.Sizeof(notifyIconData{})), hwnd: hwnd, id: 1, flags: nifMessage | nifIcon | nifTip, callbackMessage: wmTray, icon: icon}
	copy(data.tip[:], syscall.StringToUTF16("ZZZ Wallpaper Companion"))
	ok, _, addErr := shell32.NewProc("Shell_NotifyIconW").Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if ok == 0 {
		return addErr
	}
	showStartupNotification(hwnd)
	defer shell32.NewProc("Shell_NotifyIconW").Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	var message msg
	for {
		result, _, err := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return err
		}
		if result == 0 {
			return nil
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
	}
}

func showStartupNotification(hwnd uintptr) {
	data := notifyIconData{size: uint32(unsafe.Sizeof(notifyIconData{})), hwnd: hwnd, id: 1, flags: nifInfo, infoFlags: niifInfo}
	copy(data.infoTitle[:], syscall.StringToUTF16("ZZZ Wallpaper Companion"))
	copy(data.info[:], syscall.StringToUTF16("Companion is running in the notification area."))
	shell32.NewProc("Shell_NotifyIconW").Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTray:
		if lParam == wmRButtonUp {
			showTrayMenu(hwnd)
		}
		if lParam == wmLButtonDblClk {
			openURL(settingsURL(trayPort))
		}
		return 0
	case wmCommand:
		switch uint16(wParam) {
		case cmdSettings:
			openURL(settingsURL(trayPort))
		case cmdLogs:
			_ = exec.Command("explorer.exe", trayDataDir).Start()
		case cmdUpdate:
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				_ = trayResolver.Update(ctx)
			}()
		case cmdAppUpdate:
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
				defer cancel()
				result, err := trayUpdater.CheckAndStage(ctx)
				if err != nil {
					fatalDialog("Companion update check failed", err.Error())
				} else if result.UpdateStaged {
					infoDialog("Companion update ready", "Version "+result.LatestVersion+" will install when the companion restarts.")
				} else {
					infoDialog("Companion is up to date", "Version "+result.CurrentVersion+" is the latest compatible release.")
				}
			}()
		case cmdStartup:
			current := trayConfig.Get()
			if err := setLaunchOnStartup(!current.LaunchOnStartup); err != nil {
				fatalDialog("Could not update startup setting", err.Error())
			} else {
				current.LaunchOnStartup = !current.LaunchOnStartup
				if err := trayConfig.Save(current); err != nil {
					_ = setLaunchOnStartup(!current.LaunchOnStartup)
					fatalDialog("Could not save settings", err.Error())
				}
			}
		case cmdAutoUpdate:
			current := trayConfig.Get()
			current.AutoUpdate = !current.AutoUpdate
			if err := trayConfig.Save(current); err != nil {
				fatalDialog("Could not save automatic update setting", err.Error())
			}
		case cmdQuit:
			trayQuit()
			user32.NewProc("DestroyWindow").Call(hwnd)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func showTrayMenu(hwnd uintptr) {
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	appendMenuFlags(menu, 0, "Version "+version+" (build "+buildNumber+")", mfString|mfGrayed|mfDisabled)
	appendMenu(menu, cmdSettings, "Settings")
	flags := uint32(mfString)
	if trayConfig.Get().LaunchOnStartup {
		flags |= mfChecked
	}
	appendMenuFlags(menu, cmdStartup, "Launch on Windows startup", flags)
	flags = mfString
	if trayConfig.Get().AutoUpdate {
		flags |= mfChecked
	}
	appendMenuFlags(menu, cmdAutoUpdate, "Automatically download compatible updates", flags)
	appendMenu(menu, cmdAppUpdate, "Check for companion updates")
	appendMenu(menu, cmdUpdate, "Check for yt-dlp updates")
	appendMenu(menu, cmdLogs, "Open data folder")
	appendMenu(menu, cmdQuit, "Quit")
	var cursor point
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&cursor)))
	user32.NewProc("SetForegroundWindow").Call(hwnd)
	user32.NewProc("TrackPopupMenu").Call(menu, tpmRightButton, uintptr(cursor.x), uintptr(cursor.y), 0, hwnd, 0)
}

func appendMenu(menu uintptr, id uint16, label string) {
	appendMenuFlags(menu, id, label, mfString)
}

func appendMenuFlags(menu uintptr, id uint16, label string, flags uint32) {
	text, _ := syscall.UTF16PtrFromString(label)
	user32.NewProc("AppendMenuW").Call(menu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(text)))
}

func loadTrayIcon(instance uintptr) uintptr {
	icon, _, _ := user32.NewProc("LoadImageW").Call(instance, 1, imageIcon, 0, 0, lrDefaultSize)
	if icon != 0 {
		return icon
	}
	if icon := iconFromICO(trayIconICO); icon != 0 {
		return icon
	}
	icon, _, _ = user32.NewProc("LoadIconW").Call(0, idiApplication)
	return icon
}

func iconFromICO(data []byte) uintptr {
	if len(data) < 22 {
		return 0
	}
	lookup := user32.NewProc("LookupIconIdFromDirectoryEx")
	create := user32.NewProc("CreateIconFromResourceEx")
	id, _, _ := lookup.Call(uintptr(unsafe.Pointer(&data[0])), 1, 32, 32, lrDefaultSize)
	if id == 0 {
		return 0
	}
	if int(id) >= len(data) {
		return 0
	}
	iconBits := data[id:]
	icon, _, _ := create.Call(
		uintptr(unsafe.Pointer(&iconBits[0])),
		uintptr(len(iconBits)),
		1,
		0x00030000,
		32,
		32,
		0,
	)
	return icon
}

func openURL(url string) {
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

func fatalDialog(title, message string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(message)
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}

func infoDialog(title, message string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(message)
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}

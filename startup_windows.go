//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	startupKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupValue   = "ZZZWallpaperCompanion"
	keySetValue    = 0x0002
	regSZ          = 1
)

var advapi32 = syscall.NewLazyDLL("advapi32.dll")

func setLaunchOnStartup(enabled bool) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	keyPath, _ := syscall.UTF16PtrFromString(startupKeyPath)
	var key uintptr
	result, _, _ := advapi32.NewProc("RegOpenKeyExW").Call(
		uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(keyPath)), 0, keySetValue, uintptr(unsafe.Pointer(&key)),
	)
	if result != 0 {
		return fmt.Errorf("open Windows startup registry key: %w", syscall.Errno(result))
	}
	defer advapi32.NewProc("RegCloseKey").Call(key)
	name, _ := syscall.UTF16PtrFromString(startupValue)
	if !enabled {
		result, _, _ = advapi32.NewProc("RegDeleteValueW").Call(key, uintptr(unsafe.Pointer(name)))
		if result != 0 && result != uintptr(syscall.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("remove Windows startup entry: %w", syscall.Errno(result))
		}
		return nil
	}
	command := syscall.StringToUTF16(`"` + path + `"`)
	result, _, _ = advapi32.NewProc("RegSetValueExW").Call(
		key, uintptr(unsafe.Pointer(name)), 0, regSZ,
		uintptr(unsafe.Pointer(&command[0])), uintptr(len(command)*2),
	)
	if result != 0 {
		return fmt.Errorf("write Windows startup entry: %w", syscall.Errno(result))
	}
	return nil
}

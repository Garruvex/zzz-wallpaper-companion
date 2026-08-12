//go:build windows

package companion

import (
	"fmt"
	"syscall"
	"unsafe"
)

const errorAlreadyExists = 183

func acquireSingleInstance() (release func(), alreadyRunning bool, err error) {
	name, err := syscall.UTF16PtrFromString(`Local\ZZZWallpaperCompanion.SingleInstance`)
	if err != nil {
		return nil, false, err
	}
	handle, _, callErr := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").Call(
		0,
		0,
		uintptr(unsafe.Pointer(name)),
	)
	if handle == 0 {
		return nil, false, fmt.Errorf("CreateMutexW: %w", callErr)
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		_ = syscall.CloseHandle(syscall.Handle(handle))
		return func() {}, true, nil
	}
	return func() { _ = syscall.CloseHandle(syscall.Handle(handle)) }, false, nil
}

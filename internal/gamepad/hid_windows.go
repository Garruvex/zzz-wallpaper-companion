//go:build windows

package gamepad

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	setupapi                        = syscall.NewLazyDLL("setupapi.dll")
	hidDLL                          = syscall.NewLazyDLL("hid.dll")
	procSetupGetClassDevs           = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupEnumInterfaces         = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupInterfaceDetail        = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDestroy                = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procHidGetHidGuid               = hidDLL.NewProc("HidD_GetHidGuid")
	procHidGetAttributes            = hidDLL.NewProc("HidD_GetAttributes")
	procHidGetFeature               = hidDLL.NewProc("HidD_GetFeature")
	hidDeviceFlags           uint32 = 0x2 | 0x10 // DIGCF_PRESENT | DIGCF_DEVICEINTERFACE
	hidRescanEvery                  = time.Second
)

type guid struct {
	a    uint32
	b, c uint16
	d    [8]byte
}

type interfaceData struct {
	size     uint32
	class    guid
	flags    uint32
	reserved uintptr
}

type hidAttributes struct {
	size                     uint32
	vendor, product, version uint16
}

// sonyReader keeps the newest report of the first DualShock 4 or DualSense it finds. A goroutine does
// the blocking reads; the poll only takes the latest state.
type sonyReader struct {
	mu       sync.Mutex
	state    ControllerState
	running  bool
	lastScan time.Time
}

func (r *sonyReader) read() ControllerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running && time.Since(r.lastScan) >= hidRescanEvery {
		r.lastScan = time.Now()
		if handle, kind, ok := openSony(); ok {
			r.running = true
			go r.run(handle, kind)
		}
	}
	return r.state
}

func (r *sonyReader) run(handle syscall.Handle, kind sonyKind) {
	defer func() {
		syscall.CloseHandle(handle)
		r.mu.Lock()
		r.running, r.state = false, ControllerState{}
		r.mu.Unlock()
	}()
	buffer := make([]byte, 128)
	for {
		var n uint32
		if err := syscall.ReadFile(handle, buffer, &n, nil); err != nil {
			return
		}
		if state, ok := parseSony(kind, buffer[:n]); ok {
			r.mu.Lock()
			r.state = state
			r.mu.Unlock()
		}
	}
}

// openSony finds the first Sony pad on the HID bus and opens it for reading.
func openSony() (syscall.Handle, sonyKind, bool) {
	if procSetupGetClassDevs.Find() != nil || procHidGetAttributes.Find() != nil {
		return 0, notSony, false
	}
	var hidGUID guid
	procHidGetHidGuid.Call(uintptr(unsafe.Pointer(&hidGUID)))
	set, _, _ := procSetupGetClassDevs.Call(uintptr(unsafe.Pointer(&hidGUID)), 0, 0, uintptr(hidDeviceFlags))
	if set == 0 || set == ^uintptr(0) {
		return 0, notSony, false
	}
	defer procSetupDestroy.Call(set)
	for index := 0; ; index++ {
		data := interfaceData{size: uint32(unsafe.Sizeof(interfaceData{}))}
		if ok, _, _ := procSetupEnumInterfaces.Call(set, 0, uintptr(unsafe.Pointer(&hidGUID)), uintptr(index), uintptr(unsafe.Pointer(&data))); ok == 0 {
			return 0, notSony, false
		}
		var needed uint32
		procSetupInterfaceDetail.Call(set, uintptr(unsafe.Pointer(&data)), 0, 0, uintptr(unsafe.Pointer(&needed)), 0)
		if needed < 8 {
			continue
		}
		detail := make([]byte, needed)
		// The detail struct starts with its own size: 8 on 64-bit, 6 on 32-bit.
		if unsafe.Sizeof(uintptr(0)) == 8 {
			detail[0] = 8
		} else {
			detail[0] = 6
		}
		if ok, _, _ := procSetupInterfaceDetail.Call(set, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&detail[0])), uintptr(needed), 0, 0); ok == 0 {
			continue
		}
		path := (*uint16)(unsafe.Pointer(&detail[4]))
		handle, err := syscall.CreateFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		attributes := hidAttributes{size: uint32(unsafe.Sizeof(hidAttributes{}))}
		procHidGetAttributes.Call(uintptr(handle), uintptr(unsafe.Pointer(&attributes)))
		kind := sonyKindOf(attributes.vendor, attributes.product)
		if kind == notSony {
			syscall.CloseHandle(handle)
			continue
		}
		wakeBluetooth(handle, kind)
		return handle, kind, true
	}
}

// A Bluetooth DualSense sends only a short report until its calibration feature report is read; reading
// it switches the pad to the full report. Harmless over USB.
func wakeBluetooth(handle syscall.Handle, kind sonyKind) {
	if kind != kindDualSense || procHidGetFeature.Find() != nil {
		return
	}
	buffer := make([]byte, 64)
	buffer[0] = 0x05
	procHidGetFeature.Call(uintptr(handle), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
}

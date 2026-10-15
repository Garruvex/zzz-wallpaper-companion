//go:build windows

package gamepad

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

type xinputGamepad struct {
	buttons                   uint16
	leftTrigger, rightTrigger uint8
	thumbLX, thumbLY          int16
	thumbRX, thumbRY          int16
}

type xinputState struct {
	packet  uint32
	gamepad xinputGamepad
}

const (
	xDpadUp    = 0x0001
	xDpadDown  = 0x0002
	xDpadLeft  = 0x0004
	xDpadRight = 0x0008
	xStart     = 0x0010
	xBack      = 0x0020
	xLStick    = 0x0040
	xRStick    = 0x0080
	xLB        = 0x0100
	xRB        = 0x0200
	xA         = 0x1000
	xB         = 0x2000
	xX         = 0x4000
	xY         = 0x8000
	stickDead  = 16000
	maxPads    = 4
	// Asking XInput about an empty slot is slow, so empty slots are checked
	// about once a second instead of every poll.
	emptySlotEvery = 250
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetClassName        = user32.NewProc("GetClassNameW")
	procSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessage          = user32.NewProc("GetMessageW")
	procPeekMessage         = user32.NewProc("PeekMessageW")
	procPostThreadMessage   = user32.NewProc("PostThreadMessageW")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
	procXInputGetState      *syscall.LazyProc
	_                       = loadXInput()
)

func loadXInput() *syscall.LazyDLL {
	for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		dll := syscall.NewLazyDLL(name)
		if dll.Load() == nil {
			procXInputGetState = dll.NewProc("XInputGetState")
			return dll
		}
	}
	return nil
}

type winSource struct {
	mu        sync.Mutex
	connected [maxPads]bool
	tick      int
}

func NewSource() Source { return &winSource{} }

func (s *winSource) Controller() (uint32, bool) {
	state := s.DetailedController()
	return state.Buttons, state.Connected
}

func (s *winSource) DetailedController() ControllerState {
	if procXInputGetState == nil {
		return ControllerState{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tick++
	var resultState ControllerState
	for i := 0; i < maxPads; i++ {
		if !s.connected[i] && s.tick%emptySlotEvery != 1 {
			continue
		}
		var state xinputState
		result, _, _ := procXInputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&state)))
		s.connected[i] = result == 0
		if s.connected[i] {
			resultState.Buttons |= padMask(state.gamepad)
			if !resultState.Connected {
				p := state.gamepad
				resultState.DigitalButtons = digitalPadMask(p)
				resultState.Axes = [4]int16{padAxis(int32(p.thumbLX)), padAxis(-int32(p.thumbLY)), padAxis(int32(p.thumbRX)), padAxis(-int32(p.thumbRY))}
			}
			resultState.Connected = true
		}
	}
	return resultState
}

func padAxis(v int32) int16 {
	const dead = 7849
	if v >= -dead && v <= dead {
		return 0
	}
	negative := v < 0
	if negative {
		v = -v
	}
	if v > 32767 {
		v = 32767
	}
	v = (v - dead) * 32767 / (32767 - dead)
	if v < 32767 {
		v = v / 256 * 256
	}
	if negative {
		v = -v
	}
	return int16(v)
}

func digitalPadMask(p xinputGamepad) uint32 {
	// Keep positional face buttons; remove stick-to-D-pad/trigger aliases.
	digital := p
	digital.thumbLX, digital.thumbLY = 0, 0
	digital.leftTrigger, digital.rightTrigger = 0, 0
	m := padMask(digital)
	if p.leftTrigger > 30 {
		m |= 1 << ButtonL2
	}
	if p.rightTrigger > 30 {
		m |= 1 << ButtonR2
	}
	if p.buttons&xLStick != 0 {
		m |= 1 << ButtonL3
	}
	if p.buttons&xRStick != 0 {
		m |= 1 << ButtonR3
	}
	return m
}

// DesktopFocused is true when the desktop (the window the wallpaper is drawn
// behind) is the foreground window, which is what clicking the wallpaper does.
func (s *winSource) DesktopFocused() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	var name [32]uint16
	n, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	class := syscall.UTF16ToString(name[:n])
	return class == "Progman" || class == "WorkerW"
}

func (s *winSource) Keyboard() Keyboard { return theHook }

// padMask maps an Xbox pad to the Nintendo layout by position: east is A,
// south is B, west is Y and north is X.
func padMask(p xinputGamepad) uint32 {
	var m uint32
	set := func(cond bool, bit uint) {
		if cond {
			m |= 1 << bit
		}
	}
	b := p.buttons
	set(b&xDpadUp != 0 || p.thumbLY > stickDead, ButtonUp)
	set(b&xDpadDown != 0 || p.thumbLY < -stickDead, ButtonDown)
	set(b&xDpadLeft != 0 || p.thumbLX < -stickDead, ButtonLeft)
	set(b&xDpadRight != 0 || p.thumbLX > stickDead, ButtonRight)
	set(b&xB != 0, ButtonA)
	set(b&xA != 0, ButtonB)
	set(b&xX != 0, ButtonY)
	set(b&xY != 0, ButtonX)
	set(b&xLB != 0 || p.leftTrigger > 128, ButtonL)
	set(b&xRB != 0 || p.rightTrigger > 128, ButtonR)
	set(b&xStart != 0, ButtonStart)
	set(b&xBack != 0, ButtonSelect)
	return m
}

// keyHook is a low-level keyboard hook. Windows allows one callback per
// process here, so there is a single instance.
type keyHook struct {
	lifecycle sync.Mutex
	keys      keyState
	mu        sync.Mutex
	threadID  uintptr
	events    chan<- struct{}
	done      chan struct{}
}

const (
	whKeyboardLL  = 13
	wmQuit        = 0x0012
	wmKeyDown     = 0x0100
	wmSysKeyDown  = 0x0104
	llkhfInjected = 0x10
)

type kbdllHookStruct struct {
	vkCode, scanCode, flags, time uint32
	extraInfo                     uintptr
}

var (
	theHook     = &keyHook{}
	hookProcPtr = syscall.NewCallback(hookProc)
)

// modifierDown reports whether Shift, Ctrl, Alt or a Windows key is held.
func modifierDown() bool {
	for _, vk := range [...]uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C} {
		if state, _, _ := procGetAsyncKeyState.Call(vk); state&0x8000 != 0 {
			return true
		}
	}
	return false
}

func hookProc(code int, wParam uintptr, lParam uintptr) uintptr {
	if code == 0 {
		// lParam points at memory Windows owns for the duration of the call.
		event := *(**kbdllHookStruct)(unsafe.Pointer(&lParam))
		if event.flags&llkhfInjected == 0 {
			down := wParam == wmKeyDown || wParam == wmSysKeyDown
			swallow, changed := theHook.keys.handle(event.vkCode, down, modifierDown())
			if changed {
				theHook.signal()
			}
			if swallow {
				return 1
			}
		}
	}
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return result
}

func (k *keyHook) signal() {
	k.mu.Lock()
	events := k.events
	k.mu.Unlock()
	if events == nil {
		return
	}
	select {
	case events <- struct{}{}:
	default:
	}
}

// Start installs the hook on its own thread, which must pump messages for
// Windows to call it.
func (k *keyHook) Start(events chan<- struct{}) {
	k.lifecycle.Lock()
	defer k.lifecycle.Unlock()
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.done != nil {
		return
	}
	k.events = events
	k.done = make(chan struct{})
	ready := make(chan uintptr)
	go func(done chan struct{}) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		module, _, _ := procGetModuleHandle.Call(0)
		var msg [48]byte
		// Create the queue before publishing the thread ID: Stop can post
		// WM_QUIT immediately after Start returns.
		procPeekMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 0)
		hookHandle, _, _ := procSetWindowsHookEx.Call(whKeyboardLL, hookProcPtr, module, 0)
		thread, _, _ := procGetCurrentThreadID.Call()
		ready <- thread
		if hookHandle == 0 {
			return
		}
		for {
			result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if result == 0 || int32(result) == -1 {
				break
			}
		}
		procUnhookWindowsHookEx.Call(hookHandle)
	}(k.done)
	k.threadID = <-ready
}

func (k *keyHook) Stop() {
	k.lifecycle.Lock()
	defer k.lifecycle.Unlock()
	k.mu.Lock()
	done, thread := k.done, k.threadID
	k.done, k.events = nil, nil
	k.mu.Unlock()
	if done == nil {
		return
	}
	k.keys.setGrab(false)
	procPostThreadMessage.Call(thread, wmQuit, 0, 0)
	<-done
	k.keys.pause.Store(false)
	k.keys.presses.Store(0)
}

func (k *keyHook) SetGrab(on bool)   { k.keys.setGrab(on) }
func (k *keyHook) Keys() uint32      { return k.keys.held.Load() }
func (k *keyHook) PausePresses() int { return int(k.keys.presses.Swap(0)) }

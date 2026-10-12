//go:build windows

package gamepad

import (
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
	xLB        = 0x0100
	xRB        = 0x0200
	xA         = 0x1000
	xB         = 0x2000
	stickDead  = 16000
	maxPads    = 4
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procKeyState       = user32.NewProc("GetAsyncKeyState")
	xinput             = loadXInput()
	procXInputGetState *syscall.LazyProc
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

type winSource struct{}

func NewSource() Source { return winSource{} }

func (winSource) Poll(keyboard bool) (uint32, bool) {
	var mask uint32
	connected := false
	if procXInputGetState != nil {
		for i := 0; i < maxPads; i++ {
			var state xinputState
			result, _, _ := procXInputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&state)))
			if result != 0 {
				continue
			}
			connected = true
			mask |= padMask(state.gamepad)
		}
	}
	if keyboard {
		mask |= keyMask(func(vk int) bool {
			state, _, _ := procKeyState.Call(uintptr(vk))
			return state&0x8000 != 0
		})
	}
	return mask, connected
}

// padMask maps an Xbox pad to the GBA layout: the east button is GBA A and the
// south button is GBA B, as on Nintendo hardware.
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
	set(b&xLB != 0 || p.leftTrigger > 128, ButtonL)
	set(b&xRB != 0 || p.rightTrigger > 128, ButtonR)
	set(b&xStart != 0, ButtonStart)
	set(b&xBack != 0, ButtonSelect)
	return m
}

package gamepad

import "sync/atomic"

const vkPause = 0x13

// Windows virtual-key codes for the keyboard controls.
var keyBindings = map[uint32]uint{
	0x26: ButtonUp, 0x28: ButtonDown, 0x25: ButtonLeft, 0x27: ButtonRight,
	0x58: ButtonA,      // X
	0x5A: ButtonB,      // Z
	0x41: ButtonL,      // A
	0x53: ButtonR,      // S
	0x43: ButtonY,      // C (SNES Y, Genesis A)
	0x44: ButtonX,      // D (SNES X)
	0x0D: ButtonStart,  // Enter
	0x08: ButtonSelect, // Backspace
}

// keyState tracks game keys from key events. It is shared by the hook and the
// hub, so it only uses atomics.
type keyState struct {
	held    atomic.Uint32
	grab    atomic.Bool
	presses atomic.Int32
	pause   atomic.Bool // Pause is down; ignores auto-repeat
}

// handle processes one key event and reports whether to swallow it.
func (k *keyState) handle(vk uint32, down bool) (swallow, changed bool) {
	if vk == vkPause {
		if down && !k.pause.Swap(true) {
			k.presses.Add(1)
			return true, true
		}
		if !down {
			k.pause.Store(false)
		}
		return true, false
	}
	bit, ok := keyBindings[vk]
	if !ok || !k.grab.Load() {
		return false, false
	}
	if down {
		k.held.Or(1 << bit)
	} else {
		k.held.And(^uint32(1 << bit))
	}
	return true, true
}

func (k *keyState) setGrab(on bool) {
	if k.grab.Swap(on) && !on {
		k.held.Store(0)
	}
}

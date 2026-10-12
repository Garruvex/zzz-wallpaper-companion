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
	pause   atomic.Bool   // Pause is down; ignores auto-repeat
	taken   atomic.Uint32 // game keys whose key-down was swallowed
	passed  atomic.Uint32 // game keys let through because a modifier was down
}

// handle processes one key event and reports whether to swallow it.
// modified says Ctrl, Alt, Shift or a Windows key is down: those are
// shortcuts (Win+D, Ctrl+C, Alt+Enter, Shift+arrows), so they always go to
// the system and never to the game. A key is swallowed on release only if its
// press was, so the system never sees a release without a press or a press
// without a release.
func (k *keyState) handle(vk uint32, down, modified bool) (swallow, changed bool) {
	if vk == vkPause {
		if modified {
			return false, false // Win+Pause opens System, Ctrl+Break is Break
		}
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
	mask := uint32(1) << bit
	if down {
		if k.passed.Load()&mask != 0 {
			return false, false // pressed with a modifier; stays the system's until released
		}
		if modified && k.taken.Load()&mask == 0 {
			k.passed.Or(mask)
			return false, false
		}
		k.taken.Or(mask)
		k.held.Or(mask)
		return true, true
	}
	if k.passed.Load()&mask != 0 {
		k.passed.And(^mask)
		return false, false
	}
	if k.taken.Load()&mask != 0 {
		k.taken.And(^mask)
		k.held.And(^mask)
		return true, true
	}
	return false, false
}

func (k *keyState) setGrab(on bool) {
	if k.grab.Swap(on) && !on {
		k.held.Store(0)
		k.taken.Store(0)
		k.passed.Store(0)
	}
}

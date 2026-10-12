package gamepad

// Windows virtual-key codes for play-mode keyboard controls.
var keyBindings = []struct {
	vk  int
	bit uint
}{
	{0x26, ButtonUp}, {0x28, ButtonDown}, {0x25, ButtonLeft}, {0x27, ButtonRight},
	{0x58, ButtonA},      // X
	{0x5A, ButtonB},      // Z
	{0x41, ButtonL},      // A
	{0x53, ButtonR},      // S
	{0x0D, ButtonStart},  // Enter
	{0x08, ButtonSelect}, // Backspace
}

func keyMask(down func(vk int) bool) uint32 {
	var m uint32
	for _, k := range keyBindings {
		if down(k.vk) {
			m |= 1 << k.bit
		}
	}
	return m
}

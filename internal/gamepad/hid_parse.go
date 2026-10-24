package gamepad

// Sony pads (DualShock 4, DualSense) are not XInput devices, so they are read as HID reports. This
// file turns a report into the same ControllerState the XInput source gives; it has no Windows calls.

const (
	sonyVendor      = 0x054C
	dualShock4Old   = 0x05C4
	dualShock4New   = 0x09CC
	dualSense       = 0x0CE6
	dualSenseEdge   = 0x0DF2
	triggerOn       = 30
	hidStickPress   = 16000
	hatNeutral      = 8
	dsReportUSB     = 0x01
	dsReportBT      = 0x31
	ds4ReportBT     = 0x11
	sonyFaceSquare  = 0x10
	sonyFaceCross   = 0x20
	sonyFaceCircle  = 0x40
	sonyFaceTri     = 0x80
	sonyL1          = 0x01
	sonyR1          = 0x02
	sonyL2          = 0x04
	sonyR2          = 0x08
	sonyShareCreate = 0x10
	sonyOptions     = 0x20
	sonyL3          = 0x40
	sonyR3          = 0x80
)

type sonyKind int

const (
	notSony sonyKind = iota
	kindDualShock4
	kindDualSense
)

func sonyKindOf(vendor, product uint16) sonyKind {
	if vendor != sonyVendor {
		return notSony
	}
	switch product {
	case dualShock4Old, dualShock4New:
		return kindDualShock4
	case dualSense, dualSenseEdge:
		return kindDualSense
	}
	return notSony
}

// parseSony reads one input report. Face buttons are mapped by position like the Xbox pad: east is A,
// south is B, west is Y, north is X. ok is false for a report that carries no pad state.
func parseSony(kind sonyKind, report []byte) (state ControllerState, ok bool) {
	var sticks, buttons, extra, left, right int
	switch {
	case kind == kindDualSense && len(report) >= 11 && report[0] == dsReportUSB:
		sticks, buttons, extra, left, right = 1, 8, 9, 5, 6
	case kind == kindDualSense && len(report) >= 12 && report[0] == dsReportBT:
		sticks, buttons, extra, left, right = 2, 9, 10, 6, 7
	case kind == kindDualShock4 && len(report) >= 10 && report[0] == dsReportUSB:
		sticks, buttons, extra, left, right = 1, 5, 6, 8, 9
	case kind == kindDualShock4 && len(report) >= 12 && report[0] == ds4ReportBT:
		sticks, buttons, extra, left, right = 3, 7, 8, 10, 11
	default:
		return ControllerState{}, false
	}
	face, more := report[buttons], report[extra]
	lt, rt := report[left], report[right]
	hat := face & 0x0F

	var m uint32
	set := func(cond bool, bit uint) {
		if cond {
			m |= 1 << bit
		}
	}
	set(hat == 7 || hat == 0 || hat == 1, ButtonUp)
	set(hat >= 3 && hat <= 5, ButtonDown)
	set(hat >= 5 && hat <= 7, ButtonLeft)
	set(hat >= 1 && hat <= 3, ButtonRight)
	set(face&sonyFaceCircle != 0, ButtonA)
	set(face&sonyFaceCross != 0, ButtonB)
	set(face&sonyFaceSquare != 0, ButtonY)
	set(face&sonyFaceTri != 0, ButtonX)
	set(more&sonyL1 != 0, ButtonL)
	set(more&sonyR1 != 0, ButtonR)
	set(more&sonyShareCreate != 0, ButtonSelect)
	set(more&sonyOptions != 0, ButtonStart)
	set(lt > triggerOn || more&sonyL2 != 0, ButtonL2)
	set(rt > triggerOn || more&sonyR2 != 0, ButtonR2)
	set(more&sonyL3 != 0, ButtonL3)
	set(more&sonyR3 != 0, ButtonR3)
	digital := m

	axis := func(b byte) int16 { return padAxis((int32(b) - 128) * 256) }
	state.Axes = [4]int16{axis(report[sticks]), axis(report[sticks+1]), axis(report[sticks+2]), axis(report[sticks+3])}
	// The legacy mask also lets the left stick press the D-pad and a pulled trigger press the shoulder.
	legacy := m &^ (1<<ButtonL2 | 1<<ButtonR2 | 1<<ButtonL3 | 1<<ButtonR3)
	set2 := func(cond bool, bit uint) {
		if cond {
			legacy |= 1 << bit
		}
	}
	set2(state.Axes[1] < -hidStickPress, ButtonUp)
	set2(state.Axes[1] > hidStickPress, ButtonDown)
	set2(state.Axes[0] < -hidStickPress, ButtonLeft)
	set2(state.Axes[0] > hidStickPress, ButtonRight)
	set2(lt > 128, ButtonL)
	set2(rt > 128, ButtonR)
	state.Buttons, state.DigitalButtons, state.Connected = legacy, digital, true
	return state, true
}

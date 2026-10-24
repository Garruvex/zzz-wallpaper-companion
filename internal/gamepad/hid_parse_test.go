package gamepad

import "testing"

func TestDualSenseUSBReport(t *testing.T) {
	report := make([]byte, 64)
	report[0] = dsReportUSB
	copy(report[1:5], []byte{128, 128, 255, 0})
	report[5], report[6] = 255, 0
	report[8] = 8 | sonyFaceCross | sonyFaceTri
	report[9] = sonyL1 | sonyOptions | sonyL3
	state, ok := parseSony(kindDualSense, report)
	if !ok || !state.Connected {
		t.Fatal("report not read")
	}
	want := uint32(1<<ButtonB | 1<<ButtonX | 1<<ButtonL | 1<<ButtonStart | 1<<ButtonL2 | 1<<ButtonL3)
	if state.DigitalButtons != want {
		t.Fatalf("buttons %b, want %b", state.DigitalButtons, want)
	}
	if state.Axes[0] != 0 || state.Axes[2] <= 0 || state.Axes[3] >= 0 {
		t.Fatalf("axes %v", state.Axes)
	}
}

func TestSonyHatAndForeignReports(t *testing.T) {
	report := make([]byte, 64)
	report[0], report[8] = dsReportUSB, 1 // north-east
	state, _ := parseSony(kindDualSense, report)
	if state.DigitalButtons != 1<<ButtonUp|1<<ButtonRight {
		t.Fatalf("hat: %b", state.DigitalButtons)
	}
	if _, ok := parseSony(kindDualSense, []byte{0x09, 1, 2}); ok {
		t.Fatal("a short foreign report was read as pad state")
	}
	if sonyKindOf(0x045E, dualSense) != notSony || sonyKindOf(sonyVendor, dualSense) != kindDualSense || sonyKindOf(sonyVendor, dualShock4New) != kindDualShock4 {
		t.Fatal("device detection")
	}
}

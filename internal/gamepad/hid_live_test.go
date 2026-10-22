//go:build windows

package gamepad

import (
	"os"
	"testing"
	"time"
)

// Prints what a plugged-in PlayStation pad reports: ZZZ_PAD=1 go test -run Live -v
func TestLivePlayStationPad(t *testing.T) {
	if os.Getenv("ZZZ_PAD") != "1" {
		t.Skip("set ZZZ_PAD=1 with a pad connected")
	}
	source := NewSource().(*winSource)
	for i := 0; i < 20; i++ {
		state := source.DetailedController()
		t.Logf("connected=%v digital=%016b axes=%v", state.Connected, state.DigitalButtons, state.Axes)
		time.Sleep(250 * time.Millisecond)
	}
}

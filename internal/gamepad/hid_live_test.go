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

// Prints each new press the idle watcher (the PSP code) would see: ZZZ_PAD=1 go test -run Presses -v
func TestLivePressesSeenByTheIdleWatcher(t *testing.T) {
	if os.Getenv("ZZZ_PAD") != "1" {
		t.Skip("set ZZZ_PAD=1 with a pad connected")
	}
	hub := NewHub(NewSource())
	var held uint32
	hub.WatchIdle(func(buttons uint32) {
		if pressed := buttons &^ held; pressed != 0 {
			t.Logf("pressed bits %016b", pressed)
		}
		held = buttons
	})
	_, cancel := hub.Subscribe()
	defer cancel()
	time.Sleep(25 * time.Second)
}

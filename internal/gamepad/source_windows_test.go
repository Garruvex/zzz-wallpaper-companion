//go:build windows

package gamepad

import (
	"testing"
	"time"
)

func TestWindowsHookImmediateStopAndRestart(t *testing.T) {
	for i := 0; i < 30; i++ {
		theHook.Start(make(chan struct{}, 1))
		done := make(chan struct{})
		go func() { theHook.Stop(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("keyboard hook did not stop")
		}
	}
}

func TestAnalogPadKeepsDpadAndTriggersIndependent(t *testing.T) {
	p := xinputGamepad{thumbLX: 32767, thumbLY: -32768, leftTrigger: 255, buttons: xA | xLStick}
	legacy := padMask(p)
	digital := digitalPadMask(p)
	if legacy&(1<<ButtonRight) == 0 || legacy&(1<<ButtonL) == 0 {
		t.Fatal("legacy aliases changed")
	}
	if digital&((1<<ButtonRight)|(1<<ButtonDown)|(1<<ButtonL)) != 0 {
		t.Fatal("analog aliases leaked into digital mask")
	}
	if digital&(1<<ButtonL2) == 0 || digital&(1<<ButtonL3) == 0 || digital&(1<<ButtonB) == 0 {
		t.Fatal("missing digital controls")
	}
	if padAxis(7849) != 0 || padAxis(-7849) != 0 || padAxis(32768) != 32767 || padAxis(-32768) != -32767 {
		t.Fatal("axis bounds/deadzone incorrect")
	}
}

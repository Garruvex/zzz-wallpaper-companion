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

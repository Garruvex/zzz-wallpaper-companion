package gamepad

import "testing"

type analogSource struct {
	*fakeSource
	state ControllerState
}

func (s *analogSource) DetailedController() ControllerState { return s.state }

func TestAnalogSnapshotReleasesOnCaptureLoss(t *testing.T) {
	source := &analogSource{fakeSource: &fakeSource{kb: &fakeKeyboard{}, focused: true}, state: ControllerState{Connected: true, DigitalButtons: 1, Axes: [4]int16{32767, -20000, 0, 0}}}
	hub := NewHub(source)
	// Drive a deterministic step without a hardware polling goroutine.
	hub.playMode = true
	stop := make(chan struct{})
	hub.step(stop, false)
	if hub.last.DigitalButtons != 1 || hub.last.Axes[0] != 32767 || hub.last.InputVersion != 2 {
		t.Fatalf("missing analog input: %+v", hub.last)
	}
	source.focused = false
	hub.step(stop, false)
	if hub.last.Captured || hub.last.DigitalButtons != 0 || hub.last.Axes != [4]int16{} {
		t.Fatalf("input stuck after capture loss: %+v", hub.last)
	}
}

package gamepad

import (
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu       sync.Mutex
	pad, key uint32
	polled   []bool
}

func (f *fakeSource) Poll(keyboard bool) (uint32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polled = append(f.polled, keyboard)
	m := f.pad
	if keyboard {
		m |= f.key
	}
	return m, true
}

func (f *fakeSource) set(pad, key uint32) {
	f.mu.Lock()
	f.pad, f.key = pad, key
	f.mu.Unlock()
}

func next(t *testing.T, ch <-chan Snapshot, want func(Snapshot) bool) Snapshot {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case s := <-ch:
			if want(s) {
				return s
			}
		case <-timeout:
			t.Fatal("timed out waiting for snapshot")
		}
	}
}

func TestPadChangesAreBroadcast(t *testing.T) {
	src := &fakeSource{}
	hub := NewHub(src)
	ch, cancel := hub.Subscribe()
	defer cancel()
	src.set(1<<ButtonA, 0)
	s := next(t, ch, func(s Snapshot) bool { return s.Buttons == 1<<ButtonA })
	if !s.Connected {
		t.Fatal("controller should be reported connected")
	}
}

func TestKeyboardOnlyReadInPlayMode(t *testing.T) {
	src := &fakeSource{}
	hub := NewHub(src)
	ch, cancel := hub.Subscribe()
	defer cancel()
	src.set(0, 1<<ButtonStart)
	time.Sleep(60 * time.Millisecond)
	next(t, ch, func(s Snapshot) bool { return s.Buttons == 0 })
	hub.SetPlayMode(true)
	next(t, ch, func(s Snapshot) bool { return s.PlayMode && s.Buttons == 1<<ButtonStart })
	hub.SetPlayMode(false)
	next(t, ch, func(s Snapshot) bool { return !s.PlayMode && s.Buttons == 0 })
}

func TestPlayModeResetsWhenLastListenerLeaves(t *testing.T) {
	hub := NewHub(&fakeSource{})
	if hub.SetPlayMode(true).PlayMode {
		t.Fatal("play mode must not turn on with no listener")
	}
	_, cancel := hub.Subscribe()
	hub.SetPlayMode(true)
	if !hub.PlayMode() {
		t.Fatal("play mode should be on with a listener")
	}
	cancel()
	if hub.PlayMode() {
		t.Fatal("play mode should reset when the last listener leaves")
	}
}

func TestKeyMask(t *testing.T) {
	got := keyMask(func(vk int) bool { return vk == 0x58 || vk == 0x26 })
	want := uint32(1<<ButtonA | 1<<ButtonUp)
	if got != want {
		t.Fatalf("got %b want %b", got, want)
	}
}

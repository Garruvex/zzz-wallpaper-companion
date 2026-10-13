package gamepad

import (
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu      sync.Mutex
	pad     uint32
	focused bool
	kb      *fakeKeyboard
}

func (f *fakeSource) Controller() (uint32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pad, true
}

func (f *fakeSource) DesktopFocused() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.focused
}

func (f *fakeSource) Keyboard() Keyboard { return f.kb }

func (f *fakeSource) set(update func(*fakeSource)) {
	f.mu.Lock()
	update(f)
	f.mu.Unlock()
}

type fakeKeyboard struct {
	mu      sync.Mutex
	running bool
	grab    bool
	keys    uint32
	presses int
}

func (k *fakeKeyboard) Start(chan<- struct{}) { k.mu.Lock(); k.running = true; k.mu.Unlock() }
func (k *fakeKeyboard) Stop()                 { k.mu.Lock(); k.running = false; k.mu.Unlock() }
func (k *fakeKeyboard) SetGrab(on bool)       { k.mu.Lock(); k.grab = on; k.mu.Unlock() }
func (k *fakeKeyboard) Keys() uint32 {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.grab {
		return 0
	}
	return k.keys
}
func (k *fakeKeyboard) PausePresses() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := k.presses
	k.presses = 0
	return n
}
func (k *fakeKeyboard) press() { k.mu.Lock(); k.presses++; k.mu.Unlock() }
func (k *fakeKeyboard) isRunning() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.running
}

func newTestHub() (*Hub, *fakeSource) {
	src := &fakeSource{kb: &fakeKeyboard{}}
	return NewHub(src), src
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

func TestInputOnlyReachesTheGameWhileCaptured(t *testing.T) {
	hub, src := newTestHub()
	ch, cancel := hub.Subscribe()
	defer cancel()
	src.set(func(s *fakeSource) { s.pad = 1 << ButtonA; s.kb.keys = 1 << ButtonStart; s.focused = true })
	s := next(t, ch, func(s Snapshot) bool { return s.Connected })
	if s.Buttons != 0 || s.Captured {
		t.Fatalf("input leaked without play mode: %+v", s)
	}
	hub.SetPlayMode(true, CaptureDesktop)
	next(t, ch, func(s Snapshot) bool { return s.Captured && s.Buttons == 1<<ButtonA|1<<ButtonStart })
	if !src.kb.isRunning() {
		t.Fatal("key hook should be installed in play mode")
	}
	hub.SetPlayMode(false, CaptureDesktop)
	next(t, ch, func(s Snapshot) bool { return !s.Captured && s.Buttons == 0 })
	time.Sleep(20 * time.Millisecond)
	if src.kb.isRunning() {
		t.Fatal("key hook should be removed when play mode ends")
	}
}

func TestDesktopModeFollowsFocusAndPauseKey(t *testing.T) {
	hub, src := newTestHub()
	ch, cancel := hub.Subscribe()
	defer cancel()
	src.set(func(s *fakeSource) { s.focused = true })
	hub.SetPlayMode(true, CaptureDesktop)
	next(t, ch, func(s Snapshot) bool { return s.Captured })
	src.set(func(s *fakeSource) { s.focused = false })
	next(t, ch, func(s Snapshot) bool { return !s.Captured })
	src.kb.press() // Pause plays from another window
	next(t, ch, func(s Snapshot) bool { return s.Captured })
	src.kb.press()
	next(t, ch, func(s Snapshot) bool { return !s.Captured })
	src.set(func(s *fakeSource) { s.focused = true })
	next(t, ch, func(s Snapshot) bool { return s.Captured })
}

func TestDesktopModeStartsPausedWhenAnotherWindowHasFocus(t *testing.T) {
	hub, src := newTestHub()
	ch, cancel := hub.Subscribe()
	defer cancel()
	hub.SetPlayMode(true, CaptureAlways)
	next(t, ch, func(s Snapshot) bool { return s.Captured })
	hub.SetPlayMode(true, CaptureDesktop)
	next(t, ch, func(s Snapshot) bool { return s.PlayMode && !s.Captured })
	src.set(func(s *fakeSource) { s.focused = true })
	next(t, ch, func(s Snapshot) bool { return s.Captured })
}

func TestKeyModeWaitsForPause(t *testing.T) {
	hub, src := newTestHub()
	ch, cancel := hub.Subscribe()
	defer cancel()
	src.set(func(s *fakeSource) { s.focused = true })
	hub.SetPlayMode(true, CaptureKey)
	next(t, ch, func(s Snapshot) bool { return s.PlayMode && !s.Captured })
	src.kb.press()
	next(t, ch, func(s Snapshot) bool { return s.Captured })
}

func TestAlwaysModeIgnoresFocusAndPause(t *testing.T) {
	hub, src := newTestHub()
	ch, cancel := hub.Subscribe()
	defer cancel()
	hub.SetPlayMode(true, CaptureAlways)
	next(t, ch, func(s Snapshot) bool { return s.Captured })
	src.kb.press()
	select {
	case s := <-ch:
		if !s.Captured {
			t.Fatal("Pause must not release capture in always mode")
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPlayModeResetsWhenLastListenerLeaves(t *testing.T) {
	hub, src := newTestHub()
	if hub.SetPlayMode(true, CaptureDesktop).PlayMode {
		t.Fatal("play mode must not turn on with no listener")
	}
	_, cancel := hub.Subscribe()
	hub.SetPlayMode(true, CaptureDesktop)
	if !hub.PlayMode() {
		t.Fatal("play mode should be on with a listener")
	}
	cancel()
	time.Sleep(20 * time.Millisecond)
	if hub.PlayMode() || src.kb.isRunning() {
		t.Fatal("play mode and the key hook should reset when the last listener leaves")
	}
}

func TestKeyStateSwallowsOnlyGameKeysWhileGrabbing(t *testing.T) {
	var k keyState
	if swallow, _ := k.handle(0x58, true, false); swallow {
		t.Fatal("X must pass through when not grabbing")
	}
	k.setGrab(true)
	if swallow, _ := k.handle(0x58, true, false); !swallow || k.held.Load() != 1<<ButtonA {
		t.Fatal("X should be taken for the game while grabbing")
	}
	if swallow, _ := k.handle(0x51, true, false); swallow {
		t.Fatal("Q is not a game key and must pass through")
	}
	k.handle(0x58, false, false)
	if k.held.Load() != 0 {
		t.Fatal("key up should release")
	}
	k.handle(vkPause, true, false)
	k.handle(vkPause, true, false) // auto-repeat
	k.handle(vkPause, false, false)
	if k.presses.Load() != 1 {
		t.Fatalf("pause presses = %d, want 1", k.presses.Load())
	}
	k.handle(0x26, true, false)
	k.setGrab(false)
	if k.held.Load() != 0 {
		t.Fatal("releasing the grab should drop held keys")
	}
}

func TestShortcutsWithModifiersAlwaysPassThrough(t *testing.T) {
	var k keyState
	k.setGrab(true)
	// Ctrl+C on the desktop: C is a game key, but this is a shortcut.
	if swallow, _ := k.handle(0x43, true, true); swallow || k.held.Load() != 0 {
		t.Fatal("Ctrl+C must reach the system and not the game")
	}
	if swallow, _ := k.handle(0x43, true, false); swallow {
		t.Fatal("repeats of a key pressed with a modifier stay with the system")
	}
	if swallow, _ := k.handle(0x43, false, false); swallow {
		t.Fatal("its release goes to the system too")
	}
	// A game key held before the modifier is released as it was pressed.
	k.handle(0x27, true, false)
	if swallow, _ := k.handle(0x27, true, true); !swallow {
		t.Fatal("a key already taken stays taken until released")
	}
	if swallow, changed := k.handle(0x27, false, true); !swallow || !changed || k.held.Load() != 0 {
		t.Fatal("a taken key's release is swallowed and clears it")
	}
	// A release the hook never saw the press for passes through.
	if swallow, _ := k.handle(0x28, false, false); swallow {
		t.Fatal("a stray key-up must pass through")
	}
	// Win+Pause is the system's; plain Pause is ours.
	if swallow, _ := k.handle(vkPause, true, true); swallow || k.presses.Load() != 0 {
		t.Fatal("Win+Pause must pass through")
	}
}

// A reconnect must not install a replacement hook before the old pump has
// finished removing its hook.
type slowStopKeyboard struct {
	fakeKeyboard
	stopping chan struct{}
	release  chan struct{}
	starts   chan struct{}
}

func (k *slowStopKeyboard) Start(events chan<- struct{}) {
	k.fakeKeyboard.Start(events)
	k.starts <- struct{}{}
}
func (k *slowStopKeyboard) Stop() { close(k.stopping); <-k.release; k.fakeKeyboard.Stop() }

type reconnectSource struct{ kb *slowStopKeyboard }

func (s *reconnectSource) Controller() (uint32, bool) { return 0, false }
func (s *reconnectSource) DesktopFocused() bool       { return true }
func (s *reconnectSource) Keyboard() Keyboard         { return s.kb }
func TestReconnectWaitsForOldKeyboardShutdown(t *testing.T) {
	kb := &slowStopKeyboard{stopping: make(chan struct{}), release: make(chan struct{}), starts: make(chan struct{}, 2)}
	hub := NewHub(&reconnectSource{kb: kb})
	_, cancel := hub.Subscribe()
	hub.SetPlayMode(true, CaptureDesktop)
	select {
	case <-kb.starts:
	case <-time.After(time.Second):
		t.Fatal("first hook did not start")
	}
	cancel()
	select {
	case <-kb.stopping:
	case <-time.After(time.Second):
		t.Fatal("first hook did not stop")
	}
	_, cancelNext := hub.Subscribe()
	hub.SetPlayMode(true, CaptureDesktop)
	select {
	case <-kb.starts:
		t.Fatal("replacement started before shutdown finished")
	case <-time.After(30 * time.Millisecond):
	}
	// No second hook is needed for this assertion; disable before releasing.
	hub.SetPlayMode(false, CaptureDesktop)
	close(kb.release)
	cancelNext()
}

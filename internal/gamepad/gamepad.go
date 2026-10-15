// Package gamepad turns controller and keyboard state into a stream of
// RetroPad button bitmasks for the wallpaper's emulator channel.
package gamepad

import (
	"sync"
	"time"
)

// Button bit positions match the RetroPad ids used by the emulator, so the
// wallpaper can pass them straight through.
const (
	ButtonB      = 0
	ButtonY      = 1
	ButtonSelect = 2
	ButtonStart  = 3
	ButtonUp     = 4
	ButtonDown   = 5
	ButtonLeft   = 6
	ButtonRight  = 7
	ButtonA      = 8
	ButtonX      = 9
	ButtonL      = 10
	ButtonR      = 11
	ButtonL2     = 12
	ButtonR2     = 13
	ButtonL3     = 14
	ButtonR3     = 15
)

// CaptureMode decides when the game takes the keyboard and controller.
type CaptureMode string

const (
	// CaptureDesktop captures while the desktop has focus; the Pause key
	// toggles it from anywhere.
	CaptureDesktop CaptureMode = "desktop"
	// CaptureKey captures only after the Pause key turns it on.
	CaptureKey CaptureMode = "key"
	// CaptureAlways captures whenever play mode is on.
	CaptureAlways CaptureMode = "always"
)

func ParseCaptureMode(value string) (CaptureMode, bool) {
	switch mode := CaptureMode(value); mode {
	case CaptureDesktop, CaptureKey, CaptureAlways:
		return mode, true
	case "":
		return CaptureDesktop, true
	}
	return "", false
}

// Snapshot is what a subscriber receives whenever something changes.
type Snapshot struct {
	Buttons uint32 `json:"buttons"`
	// New clients use the digital mask without legacy stick/trigger aliases.
	DigitalButtons uint32 `json:"digitalButtons"`
	// LX, LY, RX, RY; positive means right/down; -32767..32767.
	Axes         [4]int16 `json:"axes"`
	InputVersion int      `json:"inputVersion"`
	PlayMode     bool     `json:"playMode"`
	Captured     bool     `json:"captured"`
	Connected    bool     `json:"controller"`
	// At is when this state was read (Unix ms), so the wallpaper can show input lag.
	At int64 `json:"at"`
}

// Source reads the hardware.
type Source interface {
	Controller() (buttons uint32, connected bool)
	DesktopFocused() bool
	Keyboard() Keyboard
}

type ControllerState struct {
	Buttons, DigitalButtons uint32
	Axes                    [4]int16
	Connected               bool
}

type detailedSource interface{ DetailedController() ControllerState }

// Keyboard is a key hook that exists only while play mode is on.
type Keyboard interface {
	// Start installs the hook; it signals events on every game or Pause key.
	Start(events chan<- struct{})
	Stop()
	// SetGrab makes game keys go to the emulator only, not to other apps.
	SetGrab(on bool)
	// Keys returns the game keys held while grabbing.
	Keys() uint32
	// PausePresses returns how many times Pause was pressed since the last call.
	PausePresses() int
}

// Key presses arrive as events; this only paces controller and focus checks.
const pollInterval = 4 * time.Millisecond

// Hub polls a Source while at least one wallpaper is listening and fans the
// result out to all of them.
type Hub struct {
	source   Source
	keyboard Keyboard

	mu       sync.Mutex
	subs     map[chan Snapshot]struct{}
	playMode bool
	mode     CaptureMode
	paused   bool
	focused  bool
	// focusKnown is false until the first focus check after play mode starts.
	focusKnown bool
	last       Snapshot
	stop       chan struct{}
	done       chan struct{}
	wake       chan struct{}
}

func NewHub(source Source) *Hub {
	return &Hub{source: source, keyboard: source.Keyboard(), mode: CaptureDesktop, subs: make(map[chan Snapshot]struct{}), wake: make(chan struct{}, 1), last: Snapshot{InputVersion: 2}}
}

// Subscribe registers a listener. The returned channel receives the current
// state immediately, then every change. Call cancel when done.
func (h *Hub) Subscribe() (updates <-chan Snapshot, cancel func()) {
	ch := make(chan Snapshot, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if len(h.subs) == 1 {
		previous := h.done
		h.stop = make(chan struct{})
		h.done = make(chan struct{})
		go func(stop, done, previous chan struct{}) {
			defer close(done)
			if previous != nil {
				<-previous
			}
			h.run(stop)
		}(h.stop, h.done, previous)
	}
	ch <- h.last
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; !ok {
			return
		}
		delete(h.subs, ch)
		if len(h.subs) == 0 {
			// Nobody is watching, so the key hook must not stay installed.
			h.playMode = false
			close(h.stop)
			h.last = Snapshot{InputVersion: 2}
		}
	}
}

// SetPlayMode turns capture on or off for the open GBA channel.
func (h *Hub) SetPlayMode(enabled bool, mode CaptureMode) Snapshot {
	h.mu.Lock()
	if len(h.subs) == 0 {
		enabled = false
	}
	if enabled && (!h.playMode || mode != h.mode) {
		// Desktop mode starts playing if the desktop already has focus; key
		// mode waits for the Pause key.
		h.paused = mode == CaptureKey
		h.focusKnown = false
	}
	h.playMode, h.mode = enabled, mode
	h.mu.Unlock()
	h.poke()
	h.mu.Lock()
	defer h.mu.Unlock()
	next := h.last
	next.PlayMode = enabled
	return next
}

func (h *Hub) PlayMode() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.playMode
}

func (h *Hub) poke() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *Hub) run(stop <-chan struct{}) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	hooked := false
	defer func() {
		if hooked {
			h.keyboard.Stop()
		}
	}()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		case <-h.wake:
		}
		h.mu.Lock()
		wantHook := h.playMode
		h.mu.Unlock()
		if wantHook && !hooked {
			h.keyboard.Start(h.wake)
			hooked = true
		} else if !wantHook && hooked {
			h.keyboard.Stop()
			hooked = false
		}
		h.step(stop, hooked)
	}
}

// step works out whether the game has the input right now and publishes it.
func (h *Hub) step(stop <-chan struct{}, hooked bool) {
	var state ControllerState
	if source, ok := h.source.(detailedSource); ok {
		state = source.DetailedController()
	} else {
		state.Buttons, state.Connected = h.source.Controller()
		state.DigitalButtons = state.Buttons
	}
	focused := h.source.DesktopFocused()
	presses := 0
	if hooked {
		presses = h.keyboard.PausePresses()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-stop:
		return
	default:
	}
	if h.mode == CaptureDesktop && (!h.focusKnown || focused != h.focused) {
		// Clicking the desktop plays; switching to any window pauses.
		h.paused = !focused
	}
	h.focused, h.focusKnown = focused, true
	if presses%2 == 1 && h.mode != CaptureAlways {
		h.paused = !h.paused
	}
	captured := h.playMode && !h.paused
	if hooked {
		h.keyboard.SetGrab(captured)
	}
	var buttons, digital uint32
	var axes [4]int16
	if captured {
		buttons, digital, axes = state.Buttons, state.DigitalButtons, state.Axes
		if hooked {
			keys := h.keyboard.Keys()
			buttons |= keys
			digital |= keys
		}
	}
	h.publishLocked(Snapshot{Buttons: buttons, DigitalButtons: digital, Axes: axes, InputVersion: 2, PlayMode: h.playMode, Captured: captured, Connected: state.Connected})
}

func (h *Hub) publishLocked(next Snapshot) {
	next.At = h.last.At
	if next == h.last {
		return
	}
	next.At = time.Now().UnixMilli()
	h.last = next
	for ch := range h.subs {
		// Keep only the newest state if a listener is slow.
		select {
		case <-ch:
		default:
		}
		ch <- next
	}
}

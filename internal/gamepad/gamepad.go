// Package gamepad turns controller and (optionally) keyboard state into a
// stream of RetroPad button bitmasks for the wallpaper's emulator channel.
package gamepad

import (
	"sync"
	"time"
)

// Button bit positions match the RetroPad ids used by the emulator, so the
// wallpaper can pass them straight through.
const (
	ButtonB      = 0
	ButtonSelect = 2
	ButtonStart  = 3
	ButtonUp     = 4
	ButtonDown   = 5
	ButtonLeft   = 6
	ButtonRight  = 7
	ButtonA      = 8
	ButtonL      = 10
	ButtonR      = 11
)

// Snapshot is what a subscriber receives whenever something changes.
type Snapshot struct {
	Buttons   uint32 `json:"buttons"`
	PlayMode  bool   `json:"playMode"`
	Connected bool   `json:"controller"`
}

// Source reads the current state of the hardware. keyboard is true only while
// play mode is on.
type Source interface {
	Poll(keyboard bool) (buttons uint32, controller bool)
}

const pollInterval = 16 * time.Millisecond

// Hub polls a Source while at least one wallpaper is listening and fans the
// result out to all of them.
type Hub struct {
	source Source

	mu       sync.Mutex
	subs     map[chan Snapshot]struct{}
	playMode bool
	last     Snapshot
	stop     chan struct{}
}

func NewHub(source Source) *Hub {
	return &Hub{source: source, subs: make(map[chan Snapshot]struct{})}
}

// Subscribe registers a listener. The returned channel receives the current
// state immediately, then every change. Call cancel when done.
func (h *Hub) Subscribe() (updates <-chan Snapshot, cancel func()) {
	ch := make(chan Snapshot, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if len(h.subs) == 1 {
		h.stop = make(chan struct{})
		go h.run(h.stop)
	}
	ch <- h.snapshotLocked()
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; !ok {
			return
		}
		delete(h.subs, ch)
		if len(h.subs) == 0 {
			// Nobody is watching, so keyboard capture must not stay on.
			h.playMode = false
			close(h.stop)
			h.last = Snapshot{}
		}
	}
}

func (h *Hub) SetPlayMode(enabled bool) Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) == 0 {
		enabled = false
	}
	h.playMode = enabled
	return h.publishLocked(h.last.Buttons, h.last.Connected)
}

func (h *Hub) PlayMode() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.playMode
}

func (h *Hub) snapshotLocked() Snapshot {
	s := h.last
	s.PlayMode = h.playMode
	return s
}

func (h *Hub) publishLocked(buttons uint32, controller bool) Snapshot {
	next := Snapshot{Buttons: buttons, PlayMode: h.playMode, Connected: controller}
	if next == h.last {
		return next
	}
	h.last = next
	for ch := range h.subs {
		// Keep only the newest state if a listener is slow.
		select {
		case <-ch:
		default:
		}
		ch <- next
	}
	return next
}

func (h *Hub) run(stop <-chan struct{}) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			h.mu.Lock()
			keyboard := h.playMode
			h.mu.Unlock()
			buttons, controller := h.source.Poll(keyboard)
			h.mu.Lock()
			select {
			case <-stop:
			default:
				h.publishLocked(buttons, controller)
			}
			h.mu.Unlock()
		}
	}
}

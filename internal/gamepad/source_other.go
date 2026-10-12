//go:build !windows

package gamepad

type noSource struct{}

func (noSource) Controller() (uint32, bool) { return 0, false }
func (noSource) DesktopFocused() bool       { return false }
func (noSource) Keyboard() Keyboard         { return noKeyboard{} }

type noKeyboard struct{}

func (noKeyboard) Start(chan<- struct{}) {}
func (noKeyboard) Stop()                 {}
func (noKeyboard) SetGrab(bool)          {}
func (noKeyboard) Keys() uint32          { return 0 }
func (noKeyboard) PausePresses() int     { return 0 }

// NewSource returns a source that never reports input; controller and
// keyboard support are Windows-only.
func NewSource() Source { return noSource{} }

//go:build !windows

package gamepad

type noSource struct{}

func (noSource) Poll(bool) (uint32, bool) { return 0, false }

// NewSource returns a source that never reports input; controller support is
// Windows-only.
func NewSource() Source { return noSource{} }

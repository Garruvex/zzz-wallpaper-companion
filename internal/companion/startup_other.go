//go:build !windows

package companion

import "errors"

func setLaunchOnStartup(bool) error {
	return errors.New("launch on startup is only supported on Windows")
}

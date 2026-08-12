//go:build !windows

package companion

import (
	"errors"
	"log"
)

func runTray(_ *ConfigStore, _ string, _ *Resolver, _ *UpdateManager, _ func()) error {
	return errors.New("tray is only supported on Windows")
}
func fatalDialog(title, message string) { log.Printf("%s: %s", title, message) }
func infoDialog(title, message string)  { log.Printf("%s: %s", title, message) }

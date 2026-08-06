//go:build !windows

package main

import (
	"errors"
	"log"
)

func runTray(_ int, _ string, _ *Resolver, _ func()) error {
	return errors.New("tray is only supported on Windows")
}
func fatalDialog(title, message string) { log.Printf("%s: %s", title, message) }

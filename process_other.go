//go:build !windows

package main

import "os/exec"

func hideCommandWindow(_ *exec.Cmd) {}

func attachKillOnCloseJob(_ *exec.Cmd) (func(), error) {
	return func() {}, nil
}

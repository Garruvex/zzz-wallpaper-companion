//go:build !windows

package main

import "os/exec"

func hideCommandWindow(_ *exec.Cmd) {}

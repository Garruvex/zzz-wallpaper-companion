//go:build windows

package companion

import (
	"os/exec"
	"testing"
	"time"
)

func TestKillOnCloseJobTerminatesChild(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping 127.0.0.1 -n 30 >nul")
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	closeJob, err := attachKillOnCloseJob(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	closeJob()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("closing the Job Object did not terminate its child")
	}
}

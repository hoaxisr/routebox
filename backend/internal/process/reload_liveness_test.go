package process

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// Reload must report a process that died on SIGHUP (sing-box exits when the
// new config cannot start) instead of the old unconditional success.
func TestReloadReportsProcessThatExitsOnSighup(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep binary not available")
	}
	run := func(t *testing.T, cmd *exec.Cmd) *Manager {
		t.Helper()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }() // reap, so Signal(0) fails once it exits
		t.Cleanup(func() { cmd.Process.Kill(); <-done })
		pid := cmd.Process.Pid
		m := &Manager{binaryPath: sleepPath}
		m.pidFinder = func() int {
			select {
			case <-done:
				return 0
			default:
				return pid
			}
		}
		return m
	}

	t.Run("exits on SIGHUP", func(t *testing.T) {
		m := run(t, exec.Command(sleepPath, "60")) // default SIGHUP action: terminate
		err := m.Reload()
		if err == nil || !strings.Contains(err.Error(), "exited") {
			t.Fatalf("Reload = %v, want 'exited'", err)
		}
	})

	t.Run("survives SIGHUP", func(t *testing.T) {
		shPath, err := exec.LookPath("sh")
		if err != nil {
			t.Skip("sh not available")
		}
		// Ignored dispositions survive exec: sleep ignores SIGHUP.
		m := run(t, exec.Command(shPath, "-c", `trap "" HUP; exec "$0" 60`, sleepPath))
		if err := m.Reload(); err != nil {
			t.Fatalf("Reload = %v, want nil", err)
		}
		if m.pidFinder() == 0 {
			t.Fatal("process should still be alive")
		}
	})

	_ = syscall.SIGHUP
}

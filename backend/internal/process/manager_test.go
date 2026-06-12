package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExeMatchesSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{binaryPath: exe}
	if !m.exeMatches(os.Getpid()) {
		t.Fatalf("expected exeMatches to match own executable %s", exe)
	}
}

func TestExeMatchesRejectsUnrelatedProcess(t *testing.T) {
	// The test binary is not amnezia-box/sing-box: must not match even
	// though "amnezia-box" appears in the configured binary path.
	m := &Manager{binaryPath: "/usr/local/bin/amnezia-box"}
	if m.exeMatches(os.Getpid()) {
		t.Fatal("test binary must not match amnezia-box")
	}
}

func TestStartedPIDLifecycle(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep binary not available")
	}
	// Resolve symlinks so binaryPath compares against /proc/<pid>/exe exactly
	resolved, err := filepath.EvalSymlinks(sleepPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(resolved, "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	childPID := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	m := &Manager{binaryPath: resolved}
	m.setStartedPID(childPID)

	// Alive: findPID must return our child via the startedPID fast path
	if pid := m.findPID(); pid != childPID {
		t.Fatalf("findPID = %d, want started child %d", pid, childPID)
	}
	if got := m.getStartedPID(); got != childPID {
		t.Fatalf("startedPID = %d, want %d", got, childPID)
	}

	// Kill and reap: the stale startedPID must be cleared
	cmd.Process.Kill()
	cmd.Wait()

	if pid := m.findPID(); pid == childPID {
		t.Fatalf("findPID returned dead pid %d", pid)
	}
	if got := m.getStartedPID(); got != 0 {
		t.Fatalf("startedPID = %d after death, want 0 (stale cleared)", got)
	}
}

func TestExeMatchesResolvesSymlinkedBinaryPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A symlink to our own executable must exact-match /proc/self/exe
	link := filepath.Join(t.TempDir(), "linked-binary")
	if err := os.Symlink(exe, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	m := &Manager{binaryPath: link}
	if !m.exeMatches(os.Getpid()) {
		t.Fatalf("expected exeMatches to resolve symlink %s -> %s", link, exe)
	}
}

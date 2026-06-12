package process

import (
	"os"
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

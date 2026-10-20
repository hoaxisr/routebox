package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicWritesWithTheModeAndLeavesNoTmp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "f")
	if err := WriteFileAtomic(path, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	fi, _ := os.Stat(path)
	if string(got) != "hi" || fi.Mode().Perm() != 0644 {
		t.Fatalf("got %q mode %v", got, fi.Mode().Perm())
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp left behind: %v", err)
	}
}

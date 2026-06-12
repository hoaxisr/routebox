package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{"hello":"world"}`)

	if err := atomicWriteFile(path, data, 0644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("content mismatch: got %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Fatalf("perm = %o, want 0644", perm)
	}

	// No temp leftovers
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in dir, got %d", len(entries))
	}

	// Overwriting an existing file works and replaces content
	data2 := []byte(`{"v":2}`)
	if err := atomicWriteFile(path, data2, 0644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got2, _ := os.ReadFile(path)
	if !bytes.Equal(got2, data2) {
		t.Fatalf("overwrite content mismatch: got %q", got2)
	}
}

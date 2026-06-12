package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()

	// 8 timestamped backups
	for ts := int64(1000); ts < 1008; ts++ {
		name := fmt.Sprintf("config.json.%d.bak", ts)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// The draft file and the config itself must never be pruned
	if err := os.WriteFile(filepath.Join(dir, "config.json.bak"), []byte("draft"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	pruneBackups(dir, "config.json", 5)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	want := []string{
		"config.json",
		"config.json.1003.bak",
		"config.json.1004.bak",
		"config.json.1005.bak",
		"config.json.1006.bak",
		"config.json.1007.bak",
		"config.json.bak",
	}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}
}

func TestPruneBackupsUnderLimit(t *testing.T) {
	dir := t.TempDir()
	for ts := int64(1000); ts < 1003; ts++ {
		name := fmt.Sprintf("config.json.%d.bak", ts)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups(dir, "config.json", 5)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("expected 3 files untouched, got %d", len(entries))
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSave_Perms0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(map[string]interface{}{"log": map[string]interface{}{"level": "info"}}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("config.json perm = %o, want 600", fi.Mode().Perm())
	}
	// The pre-save backup must also be 0600.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".bak" {
			bfi, _ := os.Stat(filepath.Join(dir, e.Name()))
			if bfi.Mode().Perm() != 0600 {
				t.Fatalf("backup %s perm = %o, want 600", e.Name(), bfi.Mode().Perm())
			}
		}
	}
}

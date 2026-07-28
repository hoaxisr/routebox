package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/util"
)

// The config is one of several files RouteBox writes, and the panel shows a
// single read-only state for all of them. That only works if the config manager
// refuses with the SAME sentinel as every other store — one errors.Is in the API
// layer, one 409, one badge. A private sentinel here would mean the config's
// refusals are recognised and the state stores' are not.
func TestConfigReadOnlyIsTheProgramWideSentinel(t *testing.T) {
	if !errors.Is(ErrReadOnly, util.ErrReadOnly) {
		t.Fatal("config.ErrReadOnly must match util.ErrReadOnly")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0400); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(map[string]interface{}{"log": map[string]interface{}{}}); !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Save error = %v, want util.ErrReadOnly", err)
	}
}

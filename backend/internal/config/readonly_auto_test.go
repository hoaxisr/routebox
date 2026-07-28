package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The read-only mode is not a flag anyone flips: it is derived from the
// filesystem. These tests pin the three states apart — an unwritable file, a
// missing file in a writable directory (a fresh install), and an unwritable
// directory (writes go through temp+rename, so the directory matters as much
// as the file).

func TestNewManagerGoesReadOnlyWhenFileIsNotWritable(t *testing.T) {
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
	if !m.IsReadOnly() {
		t.Fatal("manager must be read-only when the config file is not writable")
	}
}

// A missing config is the fresh-install path, which production takes through
// NewEmptyManager (NewManager fails on an unreadable file by design).
func TestNewEmptyManagerStaysWritableForAMissingFileInAWritableDir(t *testing.T) {
	dir := t.TempDir()
	m := NewEmptyManager(filepath.Join(dir, "config.json"))
	if m.IsReadOnly() {
		t.Fatal("a missing file in a writable dir is a fresh install, not read-only")
	}
}

func TestNewManagerGoesReadOnlyWhenDirIsNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "etc")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsReadOnly() {
		t.Fatal("a writable file in an unwritable dir is still read-only: atomic writes need the dir")
	}
}

func TestNewEmptyManagerGoesReadOnlyWhenDirIsNotWritableAndFileIsMissing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	m := NewEmptyManager(filepath.Join(dir, "config.json"))
	if !m.IsReadOnly() {
		t.Fatal("a missing file in an unwritable dir cannot be created — read-only")
	}
}

func TestSetPathReevaluatesReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.json")
	if err := os.WriteFile(locked, []byte(`{}`), 0400); err != nil {
		t.Fatal(err)
	}
	open := filepath.Join(dir, "open.json")
	if err := os.WriteFile(open, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(locked)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsReadOnly() {
		t.Fatal("expected read-only for the locked path")
	}
	if err := m.SetPath(open); err != nil {
		t.Fatal(err)
	}
	if m.IsReadOnly() {
		t.Fatal("adopting a writable path must clear read-only mode")
	}
}

// Write attempts must fail with something callers can recognise, and the
// message must name the path so the user knows what to chmod.
func TestReadOnlyWritesReportErrReadOnlyWithThePath(t *testing.T) {
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

	for name, err := range map[string]error{
		"Save":        m.Save(map[string]interface{}{"log": map[string]interface{}{}}),
		"EnsureDraft": m.EnsureDraft(),
		"SetDraft":    m.SetDraft(map[string]interface{}{"log": map[string]interface{}{}}),
	} {
		if err == nil {
			t.Fatalf("%s: expected a read-only error, got nil", name)
		}
		if !errors.Is(err, ErrReadOnly) {
			t.Fatalf("%s: error %v does not match ErrReadOnly", name, err)
		}
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("%s: error %q does not name the config path %q", name, err, path)
		}
	}
}

// SyncAwgEndpointActive is called by interactive AWG operations, whose callers
// read (false, nil) as "nothing to change" and report success. On a read-only
// config that silently produces a peer whose secrets are stored but whose
// endpoint never reaches sing-box.
func TestSyncAwgEndpointActiveRefusesInsteadOfDeferringWhenReadOnly(t *testing.T) {
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

	changed, err := m.SyncAwgEndpointActive("awg-server", specFixture())
	if err == nil {
		t.Fatalf("read-only sync reported success (changed=%v) — the caller would show a client that does not exist", changed)
	}
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("error %v must carry ErrReadOnly", err)
	}
	if changed {
		t.Fatal("a refused sync changed nothing")
	}
}

// The writability probe leaves a temp file behind if the process is killed
// mid-check; startup cleanup must sweep it like the other write leftovers.
func TestCleanupDraftSweepsWritabilityProbeLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(dir, ".routebox-wtest-123456")
	if err := os.WriteFile(orphan, nil, 0644); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	m.CleanupDraft()

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("probe leftover %s survived cleanup (stat err: %v)", orphan, err)
	}
}

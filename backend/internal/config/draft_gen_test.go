package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSaveToDiskIfGenConflict(t *testing.T) {
	m := newTestManager(t)

	if err := m.SetDraft(map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
	}); err != nil {
		t.Fatal(err)
	}
	gen := m.GetDraftGen()
	if gen == 0 {
		t.Fatal("expected non-zero draft generation after SetDraft")
	}

	// Concurrent mutation bumps the generation
	if err := m.SetDraft(map[string]interface{}{
		"log": map[string]interface{}{"level": "debug"},
	}); err != nil {
		t.Fatal(err)
	}
	if m.GetDraftGen() == gen {
		t.Fatal("draft generation did not change after second mutation")
	}

	if err := m.SaveToDiskIfGen(gen); !errors.Is(err, ErrDraftChanged) {
		t.Fatalf("SaveToDiskIfGen(stale) = %v, want ErrDraftChanged", err)
	}

	// Current generation applies cleanly
	if err := m.SaveToDiskIfGen(m.GetDraftGen()); err != nil {
		t.Fatalf("SaveToDiskIfGen(current) = %v", err)
	}
	if m.HasDraft() {
		t.Fatal("draft should be cleared after successful apply")
	}
}

func TestDiscardDraftBumpsGen(t *testing.T) {
	m := newTestManager(t)
	if err := m.SetDraft(map[string]interface{}{"log": map[string]interface{}{}}); err != nil {
		t.Fatal(err)
	}
	gen := m.GetDraftGen()
	if err := m.DiscardDraft(); err != nil {
		t.Fatal(err)
	}
	if m.GetDraftGen() == gen {
		t.Fatal("DiscardDraft must bump draft generation")
	}
}

func TestCheckConfigSkipsWhenAbsoluteBinaryMissing(t *testing.T) {
	m := newTestManager(t)
	m.SetCheckBinaryProvider(func() string { return "/nonexistent/dir/amnezia-box" })
	valid, errs := m.CheckConfig(m.GetPath())
	if !valid || errs != nil {
		t.Fatalf("expected skip (valid=true, no errors), got valid=%v errs=%v", valid, errs)
	}
}

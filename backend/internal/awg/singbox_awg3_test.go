package awg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// awg3EnableInput is a valid submission with header protection on and every
// S-padding at the fork-required minimum of 8 (validateHPKConstraint).
func awg3EnableInput() EnableInput {
	in := singboxEnableInput()
	in.HeaderProtection = true
	in.Obf = Obfuscation{S1: 8, S2: 8, S3: 8, S4: 8, CPA: "10-20", RAT: "120"}
	return in
}

func TestSingbox_AWG3_EmitsHPKWhenEnabled(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return true })

	if err := m.Enable(context.Background(), awg3EnableInput()); err != nil {
		t.Fatal(err)
	}
	if fs.lastSpec == nil {
		t.Fatal("enable must sync a non-nil server spec")
	}
	if fs.lastSpec.HeaderProtectionKey == "" {
		t.Fatal("HeaderProtectionKey must be generated and emitted when header protection is on")
	}
	if got, want := fs.lastSpec.HeaderProtectionKey, m.store.HeaderKey(); got != want {
		t.Fatalf("spec HPK %q != stored header key %q", got, want)
	}
	if got := fs.lastSpec.Obf["content_padding_addition"]; got != "10-20" {
		t.Fatalf("content_padding_addition = %v, want 10-20", got)
	}
	if got := fs.lastSpec.Obf["rekey_after_time"]; got != "120" {
		t.Fatalf("rekey_after_time = %v, want 120", got)
	}
}

func TestSingbox_AWG3_OmittedWhenBinaryTooOld(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return false })

	err := m.Enable(context.Background(), awg3EnableInput())
	if err == nil {
		t.Fatal("enable with header protection must fail on a pre-awg3 binary")
	}
	if !strings.Contains(err.Error(), "awg3") {
		t.Fatalf("error must name the awg3 binary requirement, got: %v", err)
	}
	if fs.lastSpec != nil && fs.lastSpec.HeaderProtectionKey != "" {
		t.Fatalf("HPK must not be emitted on a pre-awg3 binary: %+v", fs.lastSpec)
	}
}

func TestSingbox_HPK_RequiresS8(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return true })

	in := awg3EnableInput()
	in.Obf.S2 = 7
	err := m.Enable(context.Background(), in)
	if err == nil {
		t.Fatal("enable must fail when header protection is on and an S field is < 8")
	}
	if !strings.Contains(err.Error(), "S2") || !strings.Contains(err.Error(), ">= 8") {
		t.Fatalf("expected the S>=8 constraint error for S2, got: %v", err)
	}
}

// Additivity: on a pre-awg3 binary NONE of the three awg3 fields may appear in
// the rendered spec, or the config load breaks on the old binary.
func TestSingbox_AWG3_FieldsOmittedWhenUnsupported(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return false })

	in := singboxEnableInput() // header protection OFF, but CPA/RAT configured
	in.Obf = Obfuscation{S1: 8, S2: 8, S3: 8, S4: 8, CPA: "10-20", RAT: "120"}
	if err := m.Enable(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if fs.lastSpec == nil {
		t.Fatal("enable must sync a non-nil server spec")
	}
	if _, ok := fs.lastSpec.Obf["content_padding_addition"]; ok {
		t.Fatal("content_padding_addition must be omitted on a pre-awg3 binary")
	}
	if _, ok := fs.lastSpec.Obf["rekey_after_time"]; ok {
		t.Fatal("rekey_after_time must be omitted on a pre-awg3 binary")
	}
	if fs.lastSpec.HeaderProtectionKey != "" {
		t.Fatal("header_protection_key must be omitted on a pre-awg3 binary")
	}
}

func TestSingbox_HPK_ExportMatchesServer(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return true })

	if err := m.Enable(context.Background(), awg3EnableInput()); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	serverHPK := fs.lastSpec.HeaderProtectionKey
	if serverHPK == "" {
		t.Fatal("server spec must carry an HPK")
	}
	ep, err := m.ClientEndpoint(sum.PublicKey, "alice", "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := ep["header_protection_key"]; got != serverHPK {
		t.Fatalf("client export HPK %v != server HPK %q", got, serverHPK)
	}
	if got := ep["content_padding_addition"]; got != "10-20" {
		t.Fatalf("client export content_padding_addition = %v, want 10-20", got)
	}
	if got := ep["rekey_after_time"]; got != "120" {
		t.Fatalf("client export rekey_after_time = %v, want 120", got)
	}
}

// ClientEndpoint on a pre-awg3 binary must strip all three fields too (the
// export could be pasted into another old binary's config).
func TestSingbox_ClientEndpointOmitsAWG3WhenUnsupported(t *testing.T) {
	m, _, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return false })
	in := singboxEnableInput()
	in.Obf = Obfuscation{S1: 8, S2: 8, S3: 8, S4: 8, CPA: "10-20", RAT: "120"}
	if err := m.Enable(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddPeer(context.Background(), "bob")
	if err != nil {
		t.Fatal(err)
	}
	ep, err := m.ClientEndpoint(sum.PublicKey, "bob", "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"header_protection_key", "content_padding_addition", "rekey_after_time"} {
		if _, ok := ep[k]; ok {
			t.Fatalf("client export must omit %q on a pre-awg3 binary", k)
		}
	}
}

// A restart must keep exporting the SAME header key: RehydrateSingbox loads it
// from the store alongside the server key.
func TestSingbox_RehydrateRestoresHeaderKey(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return true })
	if err := m.Enable(context.Background(), awg3EnableInput()); err != nil {
		t.Fatal(err)
	}
	hk := fs.lastSpec.HeaderProtectionKey

	m.mu.Lock()
	m.headerKey = "" // simulate the cold post-restart state
	m.mu.Unlock()

	m.RehydrateSingbox(awg3EnableInput(), true)
	spec := m.renderServerSpec()
	if spec == nil {
		t.Fatal("enabled rehydrate must render a non-nil spec")
	}
	if spec.HeaderProtectionKey != hk {
		t.Fatalf("rehydrated HPK %q != original %q", spec.HeaderProtectionKey, hk)
	}
}

// Bug M1 regression: RehydrateSingbox must apply the same S>=8 gate as Enable.
// The settings page persists header_protection=true BEFORE Enable validates, so
// a bad combo (S<8) can land on disk; restoring the header key for it would make
// the next sweep render a spec the fork REJECTS at config load (tunnel down).
// Rehydrate must instead drop the HPK: degraded-but-loadable beats rejected.
func TestSingbox_RehydrateGatesHPKOnS8(t *testing.T) {
	m, fs, _ := newSingboxMgr(t)
	m.SetSupportsAWG3(func() bool { return true })
	if err := m.store.SetServerKey(m.serverPriv); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetHeaderKey("persisted-hpk"); err != nil {
		t.Fatal(err)
	}

	in := awg3EnableInput()
	in.Obf.S2 = 7 // persisted invalid combo: header protection on, S2 < 8
	m.RehydrateSingbox(in, true)

	if err := m.singboxSync(); err != nil { // what the 30s sweep does
		t.Fatal(err)
	}
	if fs.lastSpec == nil {
		t.Fatal("enabled rehydrate must still render a non-nil spec")
	}
	if fs.lastSpec.HeaderProtectionKey != "" {
		t.Fatalf("rehydrate must NOT restore the HPK when S<8 (fork rejects such a config), got %q",
			fs.lastSpec.HeaderProtectionKey)
	}

	// Happy path through the same route: all S>=8 restores the persisted key.
	m.RehydrateSingbox(awg3EnableInput(), true)
	if err := m.singboxSync(); err != nil {
		t.Fatal(err)
	}
	if got := fs.lastSpec.HeaderProtectionKey; got != "persisted-hpk" {
		t.Fatalf("valid rehydrate must restore the stored HPK, got %q", got)
	}
}

// Store round-trip: header_key persists in peers.toml, and stays ABSENT from the
// file when never set (kernel peers.toml must stay byte-identical).
func TestStore_HeaderKeyPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.toml")

	s := NewStore(path)
	if err := s.SetServerKey("srv"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "header_key") {
		t.Fatal("header_key must be omitted from peers.toml when unset")
	}

	if err := s.SetHeaderKey("hpk-value"); err != nil {
		t.Fatal(err)
	}
	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s2.HeaderKey(); got != "hpk-value" {
		t.Fatalf("reloaded header key = %q, want hpk-value", got)
	}
	if got := s2.ServerKey(); got != "srv" {
		t.Fatalf("server key lost across header-key save: %q", got)
	}
}

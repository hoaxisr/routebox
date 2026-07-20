package awg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// State is the module install lifecycle.
type State string

const (
	StateNotInstalled State = "not-installed"
	StateInstalling   State = "installing"
	StateReady        State = "ready"
	StateFailed       State = "failed"
)

// Status is the observable module state.
type Status struct {
	State   State  `json:"state"`
	LastErr string `json:"last_error,omitempty"`
}

// amneziaKeyFingerprint is the FULL 40-hex OpenPGP fingerprint of the AmneziaWG
// PPA signing key, pinned to resist repo/key-server compromise. Verified against
// https://launchpad.net/~amnezia/+archive/ubuntu/ppa "Signing key"
// (4096R/75C9DD72C799870E310542E24166F2C257290828 — short id 57290828 matches the
// live box). receiveAndVerifyKey asserts the fetched key carries exactly this
// BEFORE the key is ever exported to apt's trusted keyring dir.
const amneziaKeyFingerprint = "75C9DD72C799870E310542E24166F2C257290828"

// PPA coordinates. The deb822 .sources file points apt at the PPA archive and
// is Signed-By the dedicated, fingerprint-verified keyring (never the system
// trust store, never apt-key).
const (
	amneziaPPAURI      = "https://ppa.launchpadcontent.net/amnezia/ppa/ubuntu/"
	amneziaKeyring     = "/usr/share/keyrings/amnezia-awg.gpg"
	amneziaSourcesFile = "/etc/apt/sources.list.d/amnezia-awg.sources"
	amneziaKeyserver   = "keyserver.ubuntu.com"
)

// ModuleManager installs/loads the amneziawg kernel module on demand.
type ModuleManager struct {
	run           Runner
	osReleasePath string
	// tmpDir holds the scratch keyring used to receive+verify the PPA key before
	// it is trusted. Empty => os.MkdirTemp default; tests inject a TempDir so they
	// need no root and leave no global state.
	tmpDir string
	// keyringPath / sourcesPath are the trusted keyring + deb822 .sources output
	// locations. Empty => the production /usr/share/keyrings + /etc/apt defaults;
	// tests point them at a TempDir so they need no root.
	keyringPath string
	sourcesPath string
	mu          sync.Mutex
	status      Status
}

// NewModuleManager constructs a manager. osReleasePath defaults to /etc/os-release.
func NewModuleManager(run Runner, osReleasePath string) *ModuleManager {
	if osReleasePath == "" {
		osReleasePath = "/etc/os-release"
	}
	return &ModuleManager{
		run:           run,
		osReleasePath: osReleasePath,
		keyringPath:   amneziaKeyring,
		sourcesPath:   amneziaSourcesFile,
		status:        Status{State: StateNotInstalled},
	}
}

// Status returns a copy of the current state.
func (m *ModuleManager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *ModuleManager) setState(s State) {
	m.mu.Lock()
	m.status.State = s
	m.mu.Unlock()
}

func (m *ModuleManager) fail(msg string) error {
	m.mu.Lock()
	m.status = Status{State: StateFailed, LastErr: msg}
	m.mu.Unlock()
	return errors.New(msg)
}

// Ensure installs+loads the module if absent. Idempotent; single-flight (refuses
// while installing). Debian-family only in v1. All steps are arg-vectors via the
// Runner — no shell, no `sh -c`.
func (m *ModuleManager) Ensure(ctx context.Context) error {
	// Fast path (no state change): already loaded?
	if m.loaded(ctx) {
		m.setState(StateReady)
		return nil
	}
	// Single-flight: atomically check-and-claim the installing state under ONE lock
	// hold (closes the TOCTOU window between check and set).
	m.mu.Lock()
	if m.status.State == StateInstalling {
		m.mu.Unlock()
		return errors.New("install already in progress")
	}
	m.status.State = StateInstalling
	m.mu.Unlock()

	id, idLike := m.distro()
	if !isDebianFamily(id, idLike) {
		return m.fail(fmt.Sprintf("unsupported distro %q (v1 supports Debian/Ubuntu only)", id))
	}
	codename := m.codename()
	if codename == "" {
		return m.fail("cannot determine release codename (VERSION_CODENAME absent from os-release)")
	}
	ver, err := m.unameR(ctx)
	if err != nil || ver == "" {
		return m.fail(fmt.Sprintf("uname -r: %v", err))
	}
	// gnupg2 provides gpg; dirmngr is gpg's keyserver client, required for
	// `gpg --recv-keys` (absent on clean/minimal Ubuntu — gnupg2 does not pull it
	// as a hard dependency, see issue #14); kernel headers are required for the
	// DKMS module build. software-properties-common / add-apt-repository are NO
	// LONGER used — the PPA is wired up explicitly (verified keyring + deb822
	// .sources) below.
	prep := [][]string{
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "gnupg2"},
		{"apt-get", "install", "-y", "dirmngr"},
		{"apt-get", "install", "-y", "linux-headers-" + ver},
	}
	for _, args := range prep {
		if _, stderr, err := m.run.Run(ctx, args[0], args[1:]...); err != nil {
			return m.fail(fmt.Sprintf("install step %q failed: %v %s", strings.Join(args, " "), err, stderr))
		}
	}
	// VERIFY BEFORE TRUST: receive the PPA key into a scratch keyring, assert its
	// fingerprint matches the pin, and ONLY THEN export it to the trusted keyring
	// dir. A mismatch (key-server/repo compromise) stops the install cold.
	if err := m.receiveAndVerifyKey(ctx); err != nil {
		return m.fail(err.Error())
	}
	// Wire up the PPA via an explicit deb822 .sources file Signed-By the verified
	// keyring, then install + load the module.
	if err := m.writeSources(codename); err != nil {
		return m.fail(err.Error())
	}
	post := [][]string{
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "amneziawg"},
	}
	for _, args := range post {
		if _, stderr, err := m.run.Run(ctx, args[0], args[1:]...); err != nil {
			return m.fail(fmt.Sprintf("install step %q failed: %v %s", strings.Join(args, " "), err, stderr))
		}
	}
	if _, _, err := m.run.Run(ctx, "modprobe", "amneziawg"); err != nil {
		return m.fail(fmt.Sprintf("modprobe amneziawg: %v", err))
	}
	m.setState(StateReady)
	return nil
}

// unameR returns the running kernel release (for the linux-headers package).
func (m *ModuleManager) unameR(ctx context.Context) (string, error) {
	out, _, err := m.run.Run(ctx, "uname", "-r")
	return strings.TrimSpace(out), err
}

// receiveAndVerifyKey fetches the PPA signing key into a dedicated scratch
// keyring (by FULL fingerprint), asserts the received key's fingerprint matches
// the pin, and ONLY on a match exports it (binary OpenPGP, a valid apt keyring)
// to amneziaKeyring. Verify-before-trust: a mismatch/absence fails before the
// key is ever written to the trusted keyring dir. No apt-key, no shell.
func (m *ModuleManager) receiveAndVerifyKey(ctx context.Context) error {
	dir, err := os.MkdirTemp(m.tmpDir, "amnezia-key")
	if err != nil {
		return fmt.Errorf("create scratch keyring dir: %v", err)
	}
	defer os.RemoveAll(dir)
	scratch := filepath.Join(dir, "amnezia.gpg")

	// 1. Receive by full fingerprint into the scratch keyring.
	if _, stderr, err := m.run.Run(ctx, "gpg", "--no-default-keyring",
		"--keyring", scratch, "--keyserver", amneziaKeyserver,
		"--recv-keys", amneziaKeyFingerprint); err != nil {
		return fmt.Errorf("receive PPA key: %v %s", err, stderr)
	}
	// 2. Verify the received key's fingerprint == the pin BEFORE trusting it.
	out, _, err := m.run.Run(ctx, "gpg", "--no-default-keyring",
		"--keyring", scratch, "--fingerprint")
	if err != nil {
		return fmt.Errorf("verify PPA key: %v", err)
	}
	norm := strings.ReplaceAll(strings.ToUpper(out), " ", "")
	if !strings.Contains(norm, strings.ToUpper(amneziaKeyFingerprint)) {
		return fmt.Errorf("PPA signing key fingerprint mismatch (expected %s)", amneziaKeyFingerprint)
	}
	// 3. Export the verified key to the deterministic trusted keyring path. A
	// default `gpg --export` yields binary OpenPGP, which apt accepts as a
	// Signed-By keyring directly (no dearmor pipe needed).
	if _, stderr, err := m.run.Run(ctx, "gpg", "--no-default-keyring",
		"--keyring", scratch, "--output", m.keyringPath,
		"--export", amneziaKeyFingerprint); err != nil {
		return fmt.Errorf("export PPA key to keyring: %v %s", err, stderr)
	}
	return nil
}

// writeSources writes the deb822 .sources file wiring apt to the PPA, Signed-By
// the fingerprint-verified keyring. Suite is the running release codename
// (derived from os-release), never hardcoded.
func (m *ModuleManager) writeSources(codename string) error {
	body := "Types: deb\n" +
		"URIs: " + amneziaPPAURI + "\n" +
		"Suites: " + codename + "\n" +
		"Components: main\n" +
		"Signed-By: " + m.keyringPath + "\n"
	if err := os.WriteFile(m.sourcesPath, []byte(body), 0644); err != nil {
		return fmt.Errorf("write %s: %v", m.sourcesPath, err)
	}
	return nil
}

// codename reads VERSION_CODENAME from the injected os-release (e.g. "noble").
func (m *ModuleManager) codename() string {
	data, err := os.ReadFile(m.osReleasePath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VERSION_CODENAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// loaded reports whether the module is present AND the tools are usable.
func (m *ModuleManager) loaded(ctx context.Context) bool {
	out, _, err := m.run.Run(ctx, "lsmod")
	if err != nil || !strings.Contains(out, "amneziawg") {
		return false
	}
	_, _, err = m.run.Run(ctx, "awg", "--version")
	return err == nil
}

// distro reads ID / ID_LIKE from the injected os-release.
func (m *ModuleManager) distro() (id, idLike string) {
	data, err := os.ReadFile(m.osReleasePath)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "ID="); ok {
			id = strings.Trim(v, `"`)
		}
		if v, ok := strings.CutPrefix(line, "ID_LIKE="); ok {
			idLike = strings.Trim(v, `"`)
		}
	}
	return id, idLike
}

func isDebianFamily(id, idLike string) bool {
	for _, s := range []string{id, idLike} {
		if strings.Contains(s, "debian") || strings.Contains(s, "ubuntu") {
			return true
		}
	}
	return false
}

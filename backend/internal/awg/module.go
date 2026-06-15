package awg

import (
	"context"
	"errors"
	"fmt"
	"os"
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
// live box). verifyRepoKey asserts the installed keyring carries exactly this.
const amneziaKeyFingerprint = "75C9DD72C799870E310542E24166F2C257290828"

// ModuleManager installs/loads the amneziawg kernel module on demand.
type ModuleManager struct {
	run           Runner
	osReleasePath string
	mu            sync.Mutex
	status        Status
}

// NewModuleManager constructs a manager. osReleasePath defaults to /etc/os-release.
func NewModuleManager(run Runner, osReleasePath string) *ModuleManager {
	if osReleasePath == "" {
		osReleasePath = "/etc/os-release"
	}
	return &ModuleManager{run: run, osReleasePath: osReleasePath, status: Status{State: StateNotInstalled}}
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
	ver, err := m.unameR(ctx)
	if err != nil || ver == "" {
		return m.fail(fmt.Sprintf("uname -r: %v", err))
	}
	// add-apt-repository installs the PPA's signed-by keyring (never apt-key).
	steps := [][]string{
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "software-properties-common", "python3-launchpadlib", "gnupg2"},
		{"apt-get", "install", "-y", "linux-headers-" + ver},
		{"add-apt-repository", "-y", "ppa:amnezia/ppa"},
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "amneziawg"},
	}
	for _, args := range steps {
		if _, stderr, err := m.run.Run(ctx, args[0], args[1:]...); err != nil {
			return m.fail(fmt.Sprintf("install step %q failed: %v %s", strings.Join(args, " "), err, stderr))
		}
	}
	if err := m.verifyRepoKey(ctx); err != nil {
		return m.fail(err.Error())
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

// verifyRepoKey asserts the PPA signing key in apt's keyrings carries the pinned
// fingerprint. Uses gpg over the trusted keyring dir (no apt-key). A mismatch
// (key-server/repo compromise) fails the install rather than trusting a swap.
func (m *ModuleManager) verifyRepoKey(ctx context.Context) error {
	out, _, err := m.run.Run(ctx, "gpg", "--no-default-keyring",
		"--keyring", "/etc/apt/trusted.gpg.d/amnezia-ubuntu-ppa.gpg", "--fingerprint")
	if err != nil {
		return fmt.Errorf("verify PPA key: %v", err)
	}
	norm := strings.ReplaceAll(strings.ToUpper(out), " ", "")
	if !strings.Contains(norm, strings.ToUpper(amneziaKeyFingerprint)) {
		return fmt.Errorf("PPA signing key fingerprint mismatch (expected %s)", amneziaKeyFingerprint)
	}
	return nil
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

package awg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records calls and returns scripted outputs keyed by the joined argv.
// match, if set, takes precedence over the keyed maps — used for calls whose argv
// is non-deterministic (e.g. the scratch-keyring gpg --fingerprint).
type fakeRunner struct {
	calls   [][]string
	outputs map[string]string
	errs    map[string]error
	match   func(name string, args []string) (string, bool)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]string{}, errs: map[string]error{}}
}
func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.match != nil {
		if out, ok := f.match(name, args); ok {
			return out, "", nil
		}
	}
	key := name + " " + strings.Join(args, " ")
	return f.outputs[key], "", f.errs[key]
}
func (f *fakeRunner) sawContains(sub string) bool {
	return f.indexOf(sub) >= 0
}

// indexOf returns the index of the first call whose joined argv contains sub,
// or -1 if none did — used to assert call ordering.
func (f *fakeRunner) indexOf(sub string) int {
	for i, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return i
		}
	}
	return -1
}

// errFake forces a scripted command error in tests (test-only symbol).
var errFake = errors.New("fake error")

func writeOSRelease(t *testing.T, id string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "os-release")
	os.WriteFile(p, []byte("ID="+id+"\nID_LIKE=debian\nVERSION_CODENAME=noble\n"), 0644)
	return p
}

// newTestModuleManager wires a manager whose keyring/sources/scratch paths all
// live in a TempDir, so the install path writes nothing to /usr or /etc and
// needs no root.
func newTestModuleManager(t *testing.T, f Runner, id string) *ModuleManager {
	t.Helper()
	dir := t.TempDir()
	m := NewModuleManager(f, writeOSRelease(t, id))
	m.tmpDir = dir
	m.keyringPath = filepath.Join(dir, "amnezia-awg.gpg")
	m.sourcesPath = filepath.Join(dir, "amnezia-awg.sources")
	return m
}

// fakeServeFingerprint scripts the scratch-keyring `gpg --fingerprint` call. That
// keyring path is non-deterministic (MkdirTemp), so the fake matches on the call
// shape (gpg ... --fingerprint) and serves the given fingerprint for any scratch.
func fakeServeFingerprint(f *fakeRunner, fp string) {
	f.match = func(name string, args []string) (string, bool) {
		if name == "gpg" && len(args) >= 4 && args[len(args)-1] == "--fingerprint" {
			return "pub   rsa4096 ...\n      " + fp + "\nuid   AmneziaWG PPA\n", true
		}
		return "", false
	}
}

func TestModuleFastPathReady(t *testing.T) {
	f := newFakeRunner()
	f.outputs["lsmod "] = "amneziawg 131072 0\n"        // lsmod present
	f.outputs["awg --version"] = "amneziawg-tools v1.0" // tool present
	m := NewModuleManager(f, writeOSRelease(t, "ubuntu"))
	if err := m.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if m.Status().State != StateReady {
		t.Fatalf("state = %v; want ready", m.Status().State)
	}
	if f.sawContains("apt-get") {
		t.Fatal("fast-path must NOT install when module already present")
	}
}

func TestModuleInstallsOnDebian(t *testing.T) {
	f := newFakeRunner()
	f.errs["lsmod "] = errFake // module absent -> install path
	f.outputs["uname -r"] = "6.8.0-110-generic"
	// The scratch-keyring gpg --fingerprint must see the pinned fingerprint.
	fakeServeFingerprint(f, amneziaKeyFingerprint)
	m := newTestModuleManager(t, f, "ubuntu")
	if err := m.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !f.sawContains("apt-get") || !f.sawContains("amneziawg") {
		t.Fatalf("must run apt-get install amneziawg; calls=%v", f.calls)
	}
	if !f.sawContains("linux-headers-6.8.0-110-generic") {
		t.Fatalf("must install kernel headers for the running kernel; calls=%v", f.calls)
	}
	// Receive-by-fingerprint must precede any apt install of amneziawg
	// (verify-before-trust) and we must not use add-apt-repository.
	if !f.sawContains("--recv-keys " + amneziaKeyFingerprint) {
		t.Fatalf("must receive the PPA key by full fingerprint; calls=%v", f.calls)
	}
	// dirmngr is gpg's keyserver client: on clean Ubuntu it is absent (issue #14),
	// so it must be installed BEFORE the gpg --recv-keys keyserver fetch.
	dirmngrIdx := f.indexOf("apt-get install -y dirmngr")
	recvIdx := f.indexOf("--recv-keys")
	if dirmngrIdx < 0 {
		t.Fatalf("must apt-get install dirmngr (gpg keyserver client); calls=%v", f.calls)
	}
	if dirmngrIdx > recvIdx {
		t.Fatalf("dirmngr install (call %d) must precede gpg --recv-keys (call %d); calls=%v",
			dirmngrIdx, recvIdx, f.calls)
	}
	if f.sawContains("add-apt-repository") {
		t.Fatal("must NOT use add-apt-repository (deb822/inline-key path is broken)")
	}
	// The deb822 .sources file must carry the os-release codename, not a hardcode.
	body, err := os.ReadFile(m.sourcesPath)
	if err != nil {
		t.Fatalf("read sources: %v", err)
	}
	if !strings.Contains(string(body), "Suites: noble") ||
		!strings.Contains(string(body), "Signed-By: "+m.keyringPath) {
		t.Fatalf("sources file malformed:\n%s", body)
	}
	for _, c := range f.calls { // no shell anywhere
		if c[0] == "sh" || c[0] == "bash" {
			t.Fatalf("install must not shell out: %v", c)
		}
	}
	if f.sawContains("apt-key") {
		t.Fatal("must NOT use deprecated apt-key")
	}
}

func TestModuleRejectsKeyFingerprintMismatch(t *testing.T) {
	f := newFakeRunner()
	f.errs["lsmod "] = errFake
	f.outputs["uname -r"] = "6.8.0-110-generic"
	fakeServeFingerprint(f, "DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF") // wrong fp
	m := newTestModuleManager(t, f, "ubuntu")
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("must fail when the PPA key fingerprint does not match the pin")
	}
	if m.Status().State != StateFailed {
		t.Fatalf("state = %v; want failed", m.Status().State)
	}
	// Verify-before-trust: a mismatch must STOP before installing amneziawg.
	if f.sawContains("install -y amneziawg") {
		t.Fatal("must not install amneziawg after a fingerprint mismatch")
	}
}

func TestModuleUnsupportedDistroFails(t *testing.T) {
	f := newFakeRunner()
	f.errs["lsmod "] = errFake
	// os-release with a non-Debian family: ID/ID_LIKE both lack debian|ubuntu.
	dir := t.TempDir()
	p := filepath.Join(dir, "os-release")
	os.WriteFile(p, []byte("ID=alpine\nID_LIKE=\nVERSION_CODENAME=edge\n"), 0644)
	m := NewModuleManager(f, p)
	m.tmpDir, m.keyringPath, m.sourcesPath = dir, filepath.Join(dir, "k.gpg"), filepath.Join(dir, "s.sources")
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("want failure on unsupported distro")
	}
	if m.Status().State != StateFailed {
		t.Fatalf("state = %v; want failed", m.Status().State)
	}
	if f.sawContains("apt-get") {
		t.Fatal("must not run any apt-get on an unsupported distro")
	}
}

func TestModuleSingleFlight(t *testing.T) {
	m := newTestModuleManager(t, newFakeRunner(), "ubuntu")
	m.setState(StateInstalling) // simulate in-progress
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("Ensure must refuse while installing (single-flight)")
	}
}

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
type fakeRunner struct {
	calls   [][]string
	outputs map[string]string
	errs    map[string]error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]string{}, errs: map[string]error{}}
}
func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.outputs[key], "", f.errs[key]
}
func (f *fakeRunner) sawContains(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

// errFake forces a scripted command error in tests (test-only symbol).
var errFake = errors.New("fake error")

func writeOSRelease(t *testing.T, id string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "os-release")
	os.WriteFile(p, []byte("ID="+id+"\nID_LIKE=debian\n"), 0644)
	return p
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
	// gpg fingerprint check (verifyRepoKey) must see the pinned fingerprint.
	f.outputs["gpg --no-default-keyring --keyring /etc/apt/trusted.gpg.d/amnezia-ubuntu-ppa.gpg --fingerprint"] =
		"pub   rsa4096 ...\n      " + amneziaKeyFingerprint + "\nuid   AmneziaWG PPA\n"
	m := NewModuleManager(f, writeOSRelease(t, "ubuntu"))
	if err := m.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !f.sawContains("apt-get") || !f.sawContains("amneziawg") {
		t.Fatalf("must run apt-get install amneziawg; calls=%v", f.calls)
	}
	if !f.sawContains("linux-headers-6.8.0-110-generic") {
		t.Fatalf("must install kernel headers for the running kernel; calls=%v", f.calls)
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
	f.outputs["gpg --no-default-keyring --keyring /etc/apt/trusted.gpg.d/amnezia-ubuntu-ppa.gpg --fingerprint"] =
		"pub rsa4096\n   DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF\n" // wrong fp
	m := NewModuleManager(f, writeOSRelease(t, "ubuntu"))
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("must fail when the PPA key fingerprint does not match the pin")
	}
	if m.Status().State != StateFailed {
		t.Fatalf("state = %v; want failed", m.Status().State)
	}
}

func TestModuleUnsupportedDistroFails(t *testing.T) {
	f := newFakeRunner()
	f.errs["lsmod "] = errFake
	m := NewModuleManager(f, writeOSRelease(t, "alpine"))
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("want failure on unsupported distro")
	}
	if m.Status().State != StateFailed {
		t.Fatalf("state = %v; want failed", m.Status().State)
	}
}

func TestModuleSingleFlight(t *testing.T) {
	m := NewModuleManager(newFakeRunner(), writeOSRelease(t, "ubuntu"))
	m.setState(StateInstalling) // simulate in-progress
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("Ensure must refuse while installing (single-flight)")
	}
}

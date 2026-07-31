package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSystemdReloadUsesSighupNotReload guards the reload regression: the
// amnezia-box unit has no ExecReload= (CanReload=no), so `systemctl reload`
// is rejected ("Job type reload is not applicable"). Reload must instead send
// SIGHUP, which sing-box reloads on. This test fails if anyone reverts the
// argv to `reload`.
func TestSystemdReloadUsesSighupNotReload(t *testing.T) {
	got := systemdReloadArgs("amnezia-box")
	want := []string{"kill", "--signal=SIGHUP", "amnezia-box.service"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	for _, a := range got {
		if a == "reload" {
			t.Fatal("`systemctl reload` regression: the amnezia-box unit has no ExecReload= (CanReload=no); use SIGHUP")
		}
	}
}

func TestExeMatchesSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{binaryPath: exe}
	if !m.exeMatches(os.Getpid()) {
		t.Fatalf("expected exeMatches to match own executable %s", exe)
	}
}

func TestExeMatchesRejectsUnrelatedProcess(t *testing.T) {
	// The test binary is not amnezia-box/sing-box: must not match even
	// though "amnezia-box" appears in the configured binary path.
	m := &Manager{binaryPath: "/usr/local/bin/amnezia-box"}
	if m.exeMatches(os.Getpid()) {
		t.Fatal("test binary must not match amnezia-box")
	}
}

func TestStartedPIDLifecycle(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep binary not available")
	}
	// Resolve symlinks so binaryPath compares against /proc/<pid>/exe exactly
	resolved, err := filepath.EvalSymlinks(sleepPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(resolved, "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	childPID := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	m := &Manager{binaryPath: resolved}
	m.setStartedPID(childPID)

	// Alive: findPID must return our child via the startedPID fast path
	if pid := m.findPID(); pid != childPID {
		t.Fatalf("findPID = %d, want started child %d", pid, childPID)
	}
	if got := m.getStartedPID(); got != childPID {
		t.Fatalf("startedPID = %d, want %d", got, childPID)
	}

	// Kill and reap: the stale startedPID must be cleared
	cmd.Process.Kill()
	cmd.Wait()

	if pid := m.findPID(); pid == childPID {
		t.Fatalf("findPID returned dead pid %d", pid)
	}
	if got := m.getStartedPID(); got != 0 {
		t.Fatalf("startedPID = %d after death, want 0 (stale cleared)", got)
	}
}

func TestExeMatchesResolvesSymlinkedBinaryPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A symlink to our own executable must exact-match /proc/self/exe
	link := filepath.Join(t.TempDir(), "linked-binary")
	if err := os.Symlink(exe, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	m := &Manager{binaryPath: link}
	if !m.exeMatches(os.Getpid()) {
		t.Fatalf("expected exeMatches to resolve symlink %s -> %s", link, exe)
	}
}

// TestRunVersionCacheInvalidatesOnInPlaceReplacement verifies that replacing a
// binary at the same path (same path, new mtime/size) causes runVersion to
// re-execute the binary and return the updated version string.
func TestRunVersionCacheInvalidatesOnInPlaceReplacement(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "fake-binary")

	writeScript := func(versionOutput string) {
		script := "#!/bin/sh\necho '" + versionOutput + "'\n"
		if err := os.WriteFile(binaryPath, []byte(script), 0755); err != nil {
			t.Fatalf("failed to write fake binary: %v", err)
		}
	}

	// Ensure sh is available
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	// Write first version of the script
	writeScript("fake-box version 1.0.0")

	m := &Manager{}

	v1, err := m.runVersion(binaryPath)
	if err != nil {
		t.Fatalf("runVersion v1: %v", err)
	}
	if v1 != "1.0.0" {
		t.Fatalf("runVersion v1 = %q, want %q", v1, "1.0.0")
	}

	// Replace binary in-place with a new version.
	// Sleep briefly and use os.Chtimes to guarantee a distinct mtime even on
	// filesystems with coarse (1-second) timestamp granularity.
	time.Sleep(10 * time.Millisecond)
	writeScript("fake-box version 2.0.0")
	newMtime := time.Now().Add(time.Second)
	if err := os.Chtimes(binaryPath, newMtime, newMtime); err != nil {
		t.Fatalf("failed to update mtime: %v", err)
	}

	v2, err := m.runVersion(binaryPath)
	if err != nil {
		t.Fatalf("runVersion v2: %v", err)
	}
	if v2 != "2.0.0" {
		t.Fatalf("runVersion after in-place replacement = %q, want %q (cache not invalidated)", v2, "2.0.0")
	}
}

func TestParseSupportsV2RayAPI(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{
			name: "present among many tags",
			out:  "sing-box version 1.13.13-awg2.1\n\nEnvironment: go1.25 linux/amd64\nTags: with_gvisor,with_quic,with_v2ray_api,with_utls\nRevision: abc123\n",
			want: true,
		},
		{
			name: "present alone",
			out:  "amnezia-box version 1.13.13\nTags: with_v2ray_api\n",
			want: true,
		},
		{
			name: "present with surrounding spaces in list",
			out:  "Tags: with_gvisor , with_v2ray_api , with_utls\n",
			want: true,
		},
		{
			name: "absent (older binary)",
			out:  "sing-box version 1.13.12\nTags: with_gvisor,with_quic,with_utls\n",
			want: false,
		},
		{
			name: "empty output",
			out:  "",
			want: false,
		},
		{
			name: "no tags line",
			out:  "amnezia-box version 1.13.13\nEnvironment: go1.25\n",
			want: false,
		},
		{
			name: "substring of longer tag must NOT match",
			out:  "Tags: with_gvisor,with_v2ray_api_extra,with_utls\n",
			want: false,
		},
		{
			name: "prefix of token must NOT match",
			out:  "Tags: with_v2ray,with_gvisor\n",
			want: false,
		},
		{
			name: "tag-named token elsewhere (not on Tags line) must NOT match",
			out:  "Comment: this build is with_v2ray_api compatible\nTags: with_gvisor\n",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSupportsV2RayAPI(tc.out); got != tc.want {
				t.Errorf("parseSupportsV2RayAPI(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

// TestSupportsV2RayAPIFailClosed verifies that when the binary cannot be
// executed (path points at a non-runnable file and nothing is on PATH),
// SupportsV2RayAPI fails closed by returning false.
func TestSupportsV2RayAPIFailClosed(t *testing.T) {
	// A path that exists but is not executable as a `version` command would
	// still error from exec; use a guaranteed-missing path to force the error.
	m := &Manager{binaryPath: filepath.Join(t.TempDir(), "does-not-exist")}
	// Best-effort: if a real amnezia-box/sing-box happens to be on PATH this
	// test environment could differ; skip in that rare case to avoid a false
	// failure (the unit covers the no-binary fail-closed path).
	if _, err := exec.LookPath("amnezia-box"); err == nil {
		t.Skip("amnezia-box present on PATH; fail-closed path not exercised")
	}
	if _, err := exec.LookPath("sing-box"); err == nil {
		t.Skip("sing-box present on PATH; fail-closed path not exercised")
	}
	if m.SupportsV2RayAPI() {
		t.Fatal("SupportsV2RayAPI must fail closed (false) when the binary cannot run")
	}
}

// TestSupportsV2RayAPIDetectsFromFakeBinary wires a fake `version` script and
// confirms SupportsV2RayAPI reads its Tags line end-to-end (exec + parse).
func TestSupportsV2RayAPIDetectsFromFakeBinary(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-box")
	script := "#!/bin/sh\n" +
		"echo 'fake-box version 1.13.13'\n" +
		"echo 'Tags: with_gvisor,with_v2ray_api,with_utls'\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manager{binaryPath: bin}
	if !m.SupportsV2RayAPI() {
		t.Fatal("expected SupportsV2RayAPI=true for fake binary advertising with_v2ray_api")
	}

	// Replace with a binary lacking the tag (in-place, new mtime) → must flip.
	time.Sleep(10 * time.Millisecond)
	script2 := "#!/bin/sh\n" +
		"echo 'fake-box version 1.13.12'\n" +
		"echo 'Tags: with_gvisor,with_utls'\n"
	if err := os.WriteFile(bin, []byte(script2), 0755); err != nil {
		t.Fatal(err)
	}
	newMtime := time.Now().Add(time.Second)
	if err := os.Chtimes(bin, newMtime, newMtime); err != nil {
		t.Fatal(err)
	}
	if m.SupportsV2RayAPI() {
		t.Fatal("expected SupportsV2RayAPI=false after binary downgraded to drop with_v2ray_api (cache not invalidated)")
	}
}

// BinarySupportsV2RayAPI runs an explicit (not-yet-installed) binary path —
// the update-preflight seam. Tags present -> true; absent or exec error -> false.
func TestBinarySupportsV2RayAPI(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	write := func(name, versionOutput string) string {
		p := filepath.Join(dir, name)
		script := "#!/bin/sh\nprintf '%s\\n' '" + versionOutput + "'\n"
		if err := os.WriteFile(p, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	with := write("with", "fake version 1.13.13\nTags: with_gvisor,with_v2ray_api,with_clash_api")
	without := write("without", "fake version 1.14.0-alpha.48\nTags: with_gvisor,with_clash_api")
	if !BinarySupportsV2RayAPI(with) {
		t.Error("with_v2ray_api tag must report supported")
	}
	if BinarySupportsV2RayAPI(without) {
		t.Error("missing tag must report unsupported")
	}
	if BinarySupportsV2RayAPI(filepath.Join(dir, "no-such-binary")) {
		t.Error("exec error must fail closed")
	}
}

// TestParseExecStartBinary covers the pure parsing seam behind
// getBinaryFromSystemd: extract the executable `path=` from `systemctl show -p
// ExecStart` output. os.Stat existence is enforced by the caller, not here.
func TestParseExecStartBinary(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "opt path",
			in:   "ExecStart={ path=/opt/amnezia-box/amnezia-box ; argv[]=/opt/amnezia-box/amnezia-box run -c /etc/amnezia-box/config.json ; ignore_errors=no }",
			want: "/opt/amnezia-box/amnezia-box",
		},
		{
			name: "usr local with extra flags and trailing props",
			in:   "ExecStart={ path=/usr/local/bin/amnezia-box ; argv[]=/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }",
			want: "/usr/local/bin/amnezia-box",
		},
		{
			name: "realistic run -c args",
			in:   "ExecStart={ path=/usr/bin/amnezia-box ; argv[]=/usr/bin/amnezia-box run -c /etc/amnezia-box/config.json -D /var/lib/amnezia-box ; ignore_errors=no }",
			want: "/usr/bin/amnezia-box",
		},
		{
			name: "no path= is empty",
			in:   "ExecStart={ argv[]=/usr/local/bin/amnezia-box run ; ignore_errors=no }",
			want: "",
		},
		{
			name: "malformed line",
			in:   "some totally unrelated output",
			want: "",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseExecStartBinary(tt.in); got != tt.want {
				t.Fatalf("parseExecStartBinary(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPinBinaryPathReportsWhatItReplaced: pinning is how a packaged install
// (the Docker image keeps amnezia-box on the writable volume) says where the
// binary it runs lives. Whoever pins over a detected path has to be told, since
// from then on status, version and updates all follow the pinned one.
func TestPinBinaryPathReportsWhatItReplaced(t *testing.T) {
	m := &Manager{binaryPath: "/usr/local/bin/amnezia-box"}

	if replaced := m.PinBinaryPath("/config/bin/amnezia-box"); replaced != "/usr/local/bin/amnezia-box" {
		t.Errorf("replaced = %q, want the previously detected path", replaced)
	}
	if got := m.GetBinaryPath(); got != "/config/bin/amnezia-box" {
		t.Errorf("binary path = %q, want the pinned one", got)
	}
	// Same path pinned again is not a replacement — nothing to report.
	if replaced := m.PinBinaryPath("/config/bin/amnezia-box"); replaced != "" {
		t.Errorf("re-pinning the same path reported %q, want no report", replaced)
	}
	// Empty means "not configured": detection stands, and nothing is pinned.
	unpinned := &Manager{binaryPath: "/usr/local/bin/amnezia-box"}
	if replaced := unpinned.PinBinaryPath(""); replaced != "" {
		t.Errorf("empty pin reported %q", replaced)
	}
	if got := unpinned.GetBinaryPath(); got != "/usr/local/bin/amnezia-box" {
		t.Errorf("empty pin changed the path to %q", got)
	}
	if unpinned.binaryIsPinned() {
		t.Error("empty pin must leave the manager unpinned")
	}
}

// TestPinnedBinaryDoesNotFallBackToPATH: with a pin in place, a missing binary
// must read as missing. Silently resolving another amnezia-box from PATH would
// leave the panel reporting that one's version while the updater still installs
// over the pinned path — two binaries, one of them invisible.
func TestPinnedBinaryDoesNotFallBackToPATH(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	// A runnable decoy first on PATH: the fallback would find this one.
	decoy := filepath.Join(dir, "amnezia-box")
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\necho 'fake-box version 9.9.9'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	m := &Manager{}
	m.PinBinaryPath(filepath.Join(dir, "not-installed-here"))

	v, err := m.GetVersion()
	if err == nil {
		t.Fatalf("GetVersion returned %q; a pinned-but-missing binary must be an error", v)
	}
	if got := m.GetBinaryPath(); got != filepath.Join(dir, "not-installed-here") {
		t.Errorf("the pin was overwritten by detection: %q", got)
	}
	// Unpinned, the same setup resolves the decoy — this is the behaviour the
	// pin suppresses, asserted so the test can't pass for the wrong reason.
	if v, err := (&Manager{}).GetVersion(); err != nil || !strings.Contains(v, "9.9.9") {
		t.Fatalf("unpinned lookup should have found the PATH binary; got %q, %v", v, err)
	}
}

// TestExeMatchesSurvivesTheUpdateSwap: the updater renames the running binary
// to <path>.old and moves the new one into place, then calls Restart. If the
// running process stops matching the managed path at that point, Restart reads
// "not running", skips the stop, and starts a second amnezia-box next to the
// first — two processes competing for the same inbound ports, with the update
// reported as successful. Standalone installs only (a container, or any
// non-systemd host): systemctl restart tracks the unit's cgroup instead.
func TestExeMatchesSurvivesTheUpdateSwap(t *testing.T) {
	// Needs a real ELF: /proc/<pid>/exe of a script points at the interpreter.
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}
	payload, err := os.ReadFile(sleepBin)
	if err != nil {
		t.Skip("cannot read sleep binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "amnezia-box")
	if err := os.WriteFile(bin, payload, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	pid := cmd.Process.Pid

	m := &Manager{binaryPath: bin}
	if !m.exeMatches(pid) {
		t.Fatal("the process was not recognised before the swap")
	}

	// Exactly what updates.Updater.apply does to the file underneath it.
	if err := os.Rename(bin, bin+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, payload, 0755); err != nil {
		t.Fatal(err)
	}
	if !m.exeMatches(pid) {
		t.Fatal("the running process became invisible after the binary swap — Restart would start a second one beside it")
	}
}

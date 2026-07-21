package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// copyFile copies src to dst with mode 0755.
func copyFile(t *testing.T, src, dst string) []byte {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
	return data
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// assetServer serves newBinary at /asset and a checksums.txt at /checksums.txt.
func assetServer(t *testing.T, assetName string, newBinary []byte, hash string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(newBinary)))
		w.Write(newBinary)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		// amnezia-box format: multiple "<hash>  <filename>" lines
		fmt.Fprintf(w, "%s  some-other-asset\n%s  %s\n", strings.Repeat("0", 64), hash, assetName)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSmokeTestScript(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	writeScript(t, good, `echo "fake version 1.2.3"`)
	if err := smokeTest(good); err != nil {
		t.Errorf("smokeTest(good script): %v", err)
	}
	bad := filepath.Join(dir, "bad")
	writeScript(t, bad, `exit 1`)
	if err := smokeTest(bad); err == nil {
		t.Error("smokeTest(exit 1) must fail")
	}
}

func TestCheckELF(t *testing.T) {
	dir := t.TempDir()
	elf := filepath.Join(dir, "elf")
	copyFile(t, "/bin/true", elf)
	if err := checkELF(elf); err != nil {
		t.Errorf("checkELF(/bin/true copy): %v", err)
	}
	script := filepath.Join(dir, "script")
	writeScript(t, script, `echo hi`)
	if err := checkELF(script); err == nil {
		t.Error("checkELF(sh script) must fail")
	}
}

func TestParseChecksum(t *testing.T) {
	multi := []byte("aaa111  other-file\nbbb222  my-asset\n")
	got, err := parseChecksum(multi, "my-asset")
	if err != nil || got != "bbb222" {
		t.Errorf("multi-line: got %q err %v", got, err)
	}
	single := []byte("ccc333  routebox-linux-amd64\n")
	got, err = parseChecksum(single, "routebox-linux-amd64")
	if err != nil || got != "ccc333" {
		t.Errorf("single-line: got %q err %v", got, err)
	}
	if _, err = parseChecksum(multi, "missing"); err == nil {
		t.Error("missing entry must error")
	}
}

func happyTarget(dir string, restartCount *int32, restartErr func() error) (Target, string) {
	path := filepath.Join(dir, "fake-box")
	return Target{
		Name:           "amnezia-box",
		Repo:           "hoaxisr/amnezia-box",
		AssetSuffix:    func(string) (string, bool) { return "linux-amd64", true },
		BinaryPath:     func() string { return path },
		CurrentVersion: func() (string, error) { return "1.0.0", nil },
		Restart: func() error {
			atomic.AddInt32(restartCount, 1)
			if restartErr != nil {
				return restartErr()
			}
			return nil
		},
	}, path
}

func TestApplyHappyPath(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	target, path := happyTarget(dir, &restarts, nil)
	oldBytes := copyFile(t, "/bin/true", path)

	// New binary: /bin/true content + padding so old != new
	newBytes, _ := os.ReadFile("/bin/true")
	newBytes = append(newBytes, []byte("\npad")...)
	srv := assetServer(t, "fake-asset", newBytes, sha256Hex(newBytes))

	u := NewUpdater()
	rel := ReleaseInfo{
		Version:   "2.0.0",
		AssetName: "fake-asset",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	}
	res, err := u.Apply(target, rel)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Restarting {
		t.Error("non-self target must not report restarting")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(newBytes) {
		t.Error("binary not replaced with downloaded content")
	}
	old, err := os.ReadFile(path + ".old")
	if err != nil || string(old) != string(oldBytes) {
		t.Errorf(".old backup missing or wrong: %v", err)
	}
	if atomic.LoadInt32(&restarts) != 1 {
		t.Errorf("restarts = %d, want 1", restarts)
	}
	if p := u.Progress(); p.Phase != PhaseDone || p.Error != "" {
		t.Errorf("progress = %+v, want done", p)
	}
}

func TestApplySha256Mismatch(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	target, path := happyTarget(dir, &restarts, nil)
	oldBytes := copyFile(t, "/bin/true", path)

	newBytes, _ := os.ReadFile("/bin/true")
	srv := assetServer(t, "fake-asset", newBytes, strings.Repeat("f", 64)) // wrong hash

	u := NewUpdater()
	_, err := u.Apply(target, ReleaseInfo{
		AssetName: "fake-asset",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err = %v, want sha256 mismatch", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(oldBytes) {
		t.Error("original binary must be untouched")
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Error(".new must be cleaned up")
	}
	if atomic.LoadInt32(&restarts) != 0 {
		t.Error("restart must not run on verify failure")
	}
}

func TestApplyRollbackOnSmokeFailure(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	target, path := happyTarget(dir, &restarts, nil)
	oldBytes := copyFile(t, "/bin/true", path)

	// /bin/false is a valid ELF but exits 1 → smoke test fails
	newBytes, err := os.ReadFile("/bin/false")
	if err != nil {
		t.Skip("/bin/false not readable")
	}
	srv := assetServer(t, "fake-asset", newBytes, sha256Hex(newBytes))

	u := NewUpdater()
	_, err = u.Apply(target, ReleaseInfo{
		AssetName: "fake-asset",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	})
	if err == nil {
		t.Fatal("Apply must fail when smoke test fails")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(oldBytes) {
		t.Error("original binary must be untouched after smoke failure")
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Error(".new must be cleaned up")
	}
	if p := u.Progress(); p.Phase != PhaseError {
		t.Errorf("progress phase = %s, want error", p.Phase)
	}
}

// A failing Preflight must abort BEFORE the swap: installed binary untouched,
// no restart attempted, .new cleaned up — the running service never blinks.
func TestApplyPreflightFailureAbortsBeforeSwap(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	target, path := happyTarget(dir, &restarts, nil)
	var sawPath string
	target.Preflight = func(newBin string) error {
		sawPath = newBin
		return fmt.Errorf("new binary rejects the current config: unknown field v2ray_api")
	}
	oldBytes := copyFile(t, "/bin/true", path)

	newBytes, _ := os.ReadFile("/bin/true")
	newBytes = append(newBytes, []byte("\npad")...)
	srv := assetServer(t, "fake-asset", newBytes, sha256Hex(newBytes))

	u := NewUpdater()
	_, err := u.Apply(target, ReleaseInfo{
		AssetName: "fake-asset",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "preflight") {
		t.Fatalf("err = %v, want preflight failure", err)
	}
	if sawPath != path+".new" {
		t.Errorf("preflight ran against %q, want the downloaded %q", sawPath, path+".new")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(oldBytes) {
		t.Error("installed binary must be untouched on preflight failure")
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Error(".new must be cleaned up on preflight failure")
	}
	if atomic.LoadInt32(&restarts) != 0 {
		t.Error("restart must not run on preflight failure")
	}
}

// A passing Preflight lets the update proceed to swap + restart as usual.
func TestApplyPreflightPassProceeds(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	target, path := happyTarget(dir, &restarts, nil)
	target.Preflight = func(string) error { return nil }
	copyFile(t, "/bin/true", path)

	newBytes, _ := os.ReadFile("/bin/true")
	newBytes = append(newBytes, []byte("\npad")...)
	srv := assetServer(t, "fake-asset", newBytes, sha256Hex(newBytes))

	u := NewUpdater()
	if _, err := u.Apply(target, ReleaseInfo{
		AssetName: "fake-asset",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(newBytes) {
		t.Error("binary not replaced after passing preflight")
	}
	if atomic.LoadInt32(&restarts) != 1 {
		t.Errorf("restarts = %d, want 1", restarts)
	}
}

func TestApplyRollbackOnRestartFailure(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	failFirst := func() error {
		if atomic.LoadInt32(&restarts) == 1 {
			return fmt.Errorf("boom")
		}
		return nil
	}
	target, path := happyTarget(dir, &restarts, failFirst)
	oldBytes := copyFile(t, "/bin/true", path)

	newBytes, _ := os.ReadFile("/bin/true")
	newBytes = append(newBytes, []byte("\npad")...)
	srv := assetServer(t, "fake-asset", newBytes, sha256Hex(newBytes))

	u := NewUpdater()
	_, err := u.Apply(target, ReleaseInfo{
		AssetName: "fake-asset",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	})
	if err == nil {
		t.Fatal("Apply must surface restart failure")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(oldBytes) {
		t.Error("rollback must restore the original binary")
	}
	if n := atomic.LoadInt32(&restarts); n != 2 {
		t.Errorf("restarts = %d, want 2 (failed new + rollback)", n)
	}
}

func TestApplySelfUpdateSeam(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routebox")
	copyFile(t, "/bin/true", path)
	newBytes, _ := os.ReadFile("/bin/true")
	newBytes = append(newBytes, []byte("\npad")...)
	srv := assetServer(t, "routebox-linux-amd64", newBytes, sha256Hex(newBytes))

	target := Target{
		Name:           "routebox",
		Repo:           "hoaxisr/routebox",
		AssetSuffix:    func(string) (string, bool) { return "routebox-linux-amd64", true },
		BinaryPath:     func() string { return path },
		CurrentVersion: func() (string, error) { return "0.17.0", nil },
		SelfUpdate:     true,
	}

	u := NewUpdater()
	rel := ReleaseInfo{
		AssetName: "routebox-linux-amd64",
		AssetURL:  srv.URL + "/asset",
		Sha256URL: srv.URL + "/checksums.txt",
	}

	// Self-update re-execs the running binary in-place (syscall.Exec in the
	// handler), so it restarts regardless of the unit's Restart= policy or
	// whether systemd is present at all. Apply itself must NOT exit.
	t.Setenv("INVOCATION_ID", "") // not systemd
	res, err := u.Apply(target, rel)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Restarting {
		t.Error("Restarting must be true for self-update even without INVOCATION_ID")
	}

	copyFile(t, "/bin/true", path) // reset for second run
	t.Setenv("INVOCATION_ID", "abc123")
	res, err = u.Apply(target, rel)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Restarting {
		t.Error("Restarting must be true under systemd; Apply itself must NOT exit")
	}
}

func TestApplySingleFlight(t *testing.T) {
	dir := t.TempDir()
	var restarts int32
	target, path := happyTarget(dir, &restarts, nil)
	copyFile(t, "/bin/true", path)

	binData, _ := os.ReadFile("/bin/true")
	checksum := sha256Hex(binData)

	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		<-release // block first download until second Apply is rejected
		w.Write(binData)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  a\n", checksum)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	rel := ReleaseInfo{AssetName: "a", AssetURL: srv.URL + "/asset", Sha256URL: srv.URL + "/checksums.txt"}

	u := NewUpdater()
	done := make(chan error, 1)
	go func() {
		_, err := u.Apply(target, rel)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond) // first Apply is inside download
	if _, err := u.Apply(target, rel); err != ErrBusy {
		t.Errorf("second Apply err = %v, want ErrBusy", err)
	}
	close(release)
	<-done
}

func TestRunDailyChecksRespectsSetting(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(amneziaFixture))
	}))
	t.Cleanup(srv.Close)
	c := newTestChecker(t, srv.URL, "amd64")

	var enabled atomic.Bool
	stop := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		RunDailyChecks(c, []Target{amneziaTestTarget()}, enabled.Load, 20*time.Millisecond, stop)
		close(doneCh)
	}()

	time.Sleep(100 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Errorf("disabled: hits = %d, want 0", n)
	}
	enabled.Store(true)
	time.Sleep(100 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n == 0 {
		t.Error("enabled: expected at least one check")
	}
	close(stop)
	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("RunDailyChecks did not stop")
	}
}

func TestApplyRequiresChecksum(t *testing.T) {
	orig, _ := os.ReadFile("/bin/true")

	cases := []struct {
		name   string
		target func(path string) Target
	}{
		{
			name: "self-update target",
			target: func(path string) Target {
				return Target{
					Name:           "routebox",
					Repo:           "hoaxisr/routebox",
					AssetSuffix:    func(string) (string, bool) { return "routebox-linux-amd64", true },
					BinaryPath:     func() string { return path },
					CurrentVersion: func() (string, error) { return "0.17.0", nil },
					SelfUpdate:     true,
				}
			},
		},
		{
			name: "non-self (amnezia-box) target",
			target: func(path string) Target {
				var restarts int32
				return Target{
					Name:           "amnezia-box",
					Repo:           "amnezia-vpn/amnezia-wg-tools",
					AssetSuffix:    func(string) (string, bool) { return "linux-amd64", true },
					BinaryPath:     func() string { return path },
					CurrentVersion: func() (string, error) { return "1.0.0", nil },
					Restart: func() error {
						atomic.AddInt32(&restarts, 1)
						return nil
					},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "binary")
			copyFile(t, "/bin/true", path)

			u := NewUpdater()
			// Sha256URL is empty — Apply must refuse before downloading anything
			_, err := u.Apply(tc.target(path), ReleaseInfo{
				AssetName: "some-asset",
				AssetURL:  "http://127.0.0.1:0/should-not-be-fetched",
				Sha256URL: "", // intentionally empty
			})
			if err == nil {
				t.Fatal("Apply must return an error when Sha256URL is empty")
			}
			if !strings.Contains(err.Error(), "checksum") {
				t.Errorf("error should mention 'checksum', got: %v", err)
			}
			// Verify the binary was not modified
			got, _ := os.ReadFile(path)
			if string(got) != string(orig) {
				t.Error("binary must be untouched when Apply is refused due to missing checksum")
			}
		})
	}
}

package updates

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Phase of an in-flight update.
type Phase string

const (
	PhaseIdle     Phase = "idle"
	PhaseDownload Phase = "download"
	PhaseVerify   Phase = "verify"
	PhaseSwap     Phase = "swap"
	PhaseRestart  Phase = "restart"
	PhaseDone     Phase = "done"
	PhaseError    Phase = "error"
)

// ErrBusy is returned when an Apply is already running.
var ErrBusy = errors.New("another update is already in progress")

// Progress is a snapshot of the current/last Apply, polled by the UI.
type Progress struct {
	Target          string `json:"target"`
	Phase           Phase  `json:"phase"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	Error           string `json:"error,omitempty"`
}

// ApplyResult reports self-update restart semantics. For SelfUpdate targets
// Apply NEVER exits the process itself: it returns Restarting=true when a
// supervisor (systemd) will respawn us, and the API HANDLER is responsible
// for responding to the client and scheduling os.Exit(0).
type ApplyResult struct {
	Restarting bool
}

// Updater downloads, verifies and atomically swaps target binaries.
type Updater struct {
	client  *http.Client
	applyMu sync.Mutex // single-flight across all targets

	progMu   sync.Mutex
	progress Progress
}

// NewUpdater builds an Updater. Long client timeout: release downloads on
// slow router uplinks can take minutes.
func NewUpdater() *Updater {
	return &Updater{
		client:   &http.Client{Timeout: 10 * time.Minute},
		progress: Progress{Phase: PhaseIdle},
	}
}

// Progress returns a copy of the current progress snapshot.
func (u *Updater) Progress() Progress {
	u.progMu.Lock()
	defer u.progMu.Unlock()
	return u.progress
}

func (u *Updater) setPhase(target string, phase Phase) {
	u.progMu.Lock()
	u.progress.Target = target
	u.progress.Phase = phase
	u.progress.Error = ""
	u.progMu.Unlock()
}

func (u *Updater) setError(err error) {
	u.progMu.Lock()
	u.progress.Phase = PhaseError
	u.progress.Error = err.Error()
	u.progMu.Unlock()
}

func (u *Updater) setBytes(downloaded, total int64) {
	u.progMu.Lock()
	u.progress.DownloadedBytes = downloaded
	u.progress.TotalBytes = total
	u.progMu.Unlock()
}

// Apply runs download → verify → swap → restart for one target.
func (u *Updater) Apply(t Target, rel ReleaseInfo) (ApplyResult, error) {
	if !u.applyMu.TryLock() {
		return ApplyResult{}, ErrBusy
	}
	defer u.applyMu.Unlock()

	u.setBytes(0, 0)
	res, err := u.apply(t, rel)
	if err != nil {
		u.setError(err)
		return res, err
	}
	u.setPhase(t.Name, PhaseDone)
	return res, nil
}

func (u *Updater) apply(t Target, rel ReleaseInfo) (ApplyResult, error) {
	path := t.BinaryPath()
	if path == "" {
		return ApplyResult{}, fmt.Errorf("%s: binary path unknown", t.Name)
	}
	newPath := path + ".new" // same dir → rename is atomic
	oldPath := path + ".old" // single rollback slot, overwritten each update

	// 1. Download
	u.setPhase(t.Name, PhaseDownload)
	if err := u.download(rel.AssetURL, newPath); err != nil {
		os.Remove(newPath)
		return ApplyResult{}, fmt.Errorf("download: %w", err)
	}

	// 2. Verify: sha256 (when published) + ELF magic + smoke run
	u.setPhase(t.Name, PhaseVerify)
	if rel.Sha256URL != "" {
		if err := u.verifySha256(newPath, rel); err != nil {
			os.Remove(newPath)
			return ApplyResult{}, err
		}
	}
	if err := checkELF(newPath); err != nil {
		os.Remove(newPath)
		return ApplyResult{}, err
	}
	if err := smokeTest(newPath); err != nil {
		os.Remove(newPath)
		return ApplyResult{}, fmt.Errorf("smoke test: %w", err)
	}

	// 3. Swap
	u.setPhase(t.Name, PhaseSwap)
	if err := os.Rename(path, oldPath); err != nil {
		os.Remove(newPath)
		return ApplyResult{}, fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(newPath, path); err != nil {
		os.Rename(oldPath, path) // restore
		return ApplyResult{}, fmt.Errorf("install new binary: %w", err)
	}

	// 4. Restart
	u.setPhase(t.Name, PhaseRestart)
	if t.SelfUpdate {
		return ApplyResult{Restarting: RunningUnderSystemd()}, nil
	}
	if t.Restart != nil {
		if err := t.Restart(); err != nil {
			// Auto-rollback: keep the bad binary for inspection, restore old,
			// restart again.
			os.Rename(path, newPath+".failed")
			os.Rename(oldPath, path)
			if rerr := t.Restart(); rerr != nil {
				return ApplyResult{}, fmt.Errorf(
					"restart with new binary failed (%v); rollback restart also failed: %w", err, rerr)
			}
			return ApplyResult{}, fmt.Errorf("restart with new binary failed, rolled back: %w", err)
		}
	}
	return ApplyResult{}, nil
}

// download streams url to dst (mode 0755), updating byte progress.
func (u *Updater) download(url, dst string) error {
	resp, err := u.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	total := resp.ContentLength
	u.setBytes(0, total)

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	var written int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			u.setBytes(written, total)
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// verifySha256 fetches the checksum asset and compares it with the file.
func (u *Updater) verifySha256(path string, rel ReleaseInfo) error {
	resp, err := u.client.Get(rel.Sha256URL)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch checksum: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	want, err := parseChecksum(data, rel.AssetName)
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, want)
	}
	return nil
}

// parseChecksum extracts the hash for assetName from sha256sum output.
// Handles both checksums.txt (many "<hash>  <file>" lines, amnezia-box)
// and single-file "<hash>  <file>" .sha256 assets (routebox).
func parseChecksum(data []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s", assetName)
}

// checkELF verifies the file starts with the ELF magic bytes.
func checkELF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("read ELF magic: %w", err)
	}
	if !bytes.Equal(magic, []byte{0x7f, 'E', 'L', 'F'}) {
		return fmt.Errorf("downloaded file is not an ELF binary")
	}
	return nil
}

// smokeTest runs `<binary> version` and requires exit 0.
func smokeTest(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "version").CombinedOutput(); err != nil {
		return fmt.Errorf("%s version: %w (%s)", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RunningUnderSystemd reports whether the process was started by systemd
// (which sets INVOCATION_ID) and will therefore be respawned after exit.
func RunningUnderSystemd() bool {
	return os.Getenv("INVOCATION_ID") != ""
}

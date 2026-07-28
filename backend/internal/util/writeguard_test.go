package util

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The guard is the single answer to "can RouteBox write this file?", shared by
// the sing-box config and every RouteBox state file. These tests pin the four
// states it has to tell apart: a writable file, an unwritable file, an
// unwritable directory (writes go through temp+rename, so the directory counts),
// and a path whose directory does not exist yet but can be created.

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}
}

func TestWriteGuardSeesAWritableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if NewWriteGuard(path).IsReadOnly() {
		t.Fatal("a writable file must not be reported read-only")
	}
}

func TestWriteGuardSeesAnUnwritableFile(t *testing.T) {
	skipAsRoot(t)
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(path, nil, 0400); err != nil {
		t.Fatal(err)
	}
	if !NewWriteGuard(path).IsReadOnly() {
		t.Fatal("a 0400 file must be reported read-only")
	}
}

func TestWriteGuardSeesAnUnwritableDirectory(t *testing.T) {
	skipAsRoot(t)
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.toml")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	if !NewWriteGuard(path).IsReadOnly() {
		t.Fatal("a writable file in an unwritable dir is still read-only: atomic writes need the dir")
	}
}

// Every store creates its directory on first save (MkdirAll), so a not-yet-made
// directory under a writable parent is a fresh install, not a read-only one.
// Reporting it read-only would put the badge up on every first boot.
func TestWriteGuardTreatsACreatableDirectoryAsWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox", "state.toml")
	if NewWriteGuard(path).IsReadOnly() {
		t.Fatal("a missing directory under a writable parent can be created — not read-only")
	}
}

// Persistence-disabled stores (empty path) never write, so they can never be
// blocked. Reporting them read-only would put a permanent badge up in tests and
// on any deployment that runs without a state file.
func TestWriteGuardOnAnEmptyPathIsNeverReadOnly(t *testing.T) {
	g := NewWriteGuard("")
	if g.IsReadOnly() {
		t.Fatal("an empty path means persistence is off, not read-only")
	}
	if err := g.Note(errors.New("boom")); err == nil || errors.Is(err, ErrReadOnly) {
		t.Fatalf("an empty path must pass errors through unchanged, got %v", err)
	}
}

func TestNoteClassifiesAFailedWriteOnAnUnwritablePath(t *testing.T) {
	skipAsRoot(t)
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	path := filepath.Join(dir, "state.toml")

	g := NewWriteGuard(path)
	err := g.Note(errors.New("permission denied"))
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("error %v must carry ErrReadOnly so the API can answer 409", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q must name the path %q the user has to fix", err, path)
	}
}

// A failure that is not about writability keeps its own error: a malformed
// value or a full disk is not something chmod fixes.
func TestNoteLeavesAFailureOnAWritablePathAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	boom := errors.New("encode failed")
	g := NewWriteGuard(path)
	if err := g.Note(boom); !errors.Is(err, boom) || errors.Is(err, ErrReadOnly) {
		t.Fatalf("error = %v, want the original", err)
	}
	if g.IsReadOnly() {
		t.Fatal("a failure on a writable path must not raise the read-only verdict")
	}
}

// The verdict is taken at startup and would otherwise stick for the life of the
// process. An operator who remounts rw must see the badge clear without a
// restart, so a standing read-only verdict is re-probed (a writable one never
// pays for the probe).
func TestAStandingReadOnlyVerdictIsRechecked(t *testing.T) {
	skipAsRoot(t)
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	path := filepath.Join(dir, "state.toml")

	now := time.Now()
	g := NewWriteGuard(path)
	g.now = func() time.Time { return now }
	if !g.IsReadOnly() {
		t.Fatal("harness: an unwritable dir must start read-only")
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if g.IsReadOnly() {
		t.Fatal("the verdict must be lifted once the path is writable again")
	}
}

// A successful write is proof of writability — cheaper and more direct than a
// probe, and it clears a verdict the operator has just fixed.
func TestASuccessfulWriteClearsTheVerdict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	g := NewWriteGuard(path)
	g.SetReadOnly(true)
	if err := g.Note(nil); err != nil {
		t.Fatalf("Note(nil) = %v, want nil", err)
	}
	if g.IsReadOnly() {
		t.Fatal("a write that went through must clear the read-only verdict")
	}
}

// A full disk is not a permissions problem, and "check ownership and
// permissions of the file and its directory" sends the operator looking in the
// wrong place. The probe cannot conclude anything about writability from ENOSPC,
// so it must not: the write's own error ("no space left on device") says it
// better than any verdict of ours.
func TestOutOfSpaceIsNotAWritabilityVerdict(t *testing.T) {
	for _, err := range []error{syscall.ENOSPC, syscall.EDQUOT,
		&os.PathError{Op: "open", Path: "/x", Err: syscall.ENOSPC}} {
		if meansUnwritable(err) {
			t.Fatalf("%v must not be read as unwritable — it is a full disk, not a locked one", err)
		}
	}
	for _, err := range []error{syscall.EACCES, syscall.EROFS, syscall.EPERM} {
		if !meansUnwritable(err) {
			t.Fatalf("%v must be read as unwritable", err)
		}
	}
}

func TestClassifyPassesOutOfSpaceThrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	full := &os.PathError{Op: "write", Path: path, Err: syscall.ENOSPC}
	if err := ClassifyWriteErr(path, full); !errors.Is(err, syscall.ENOSPC) || errors.Is(err, ErrReadOnly) {
		t.Fatalf("error = %v, want the original ENOSPC", err)
	}
}

// The probe writes a throwaway file and removes it. A kill in that window leaves
// one behind, and the promise has always been that the next startup sweeps it.
// That promise used to be kept by the config directory's cleanup alone, which
// says nothing about the two other directories the probe now visits — so the
// probe cleans up after itself, wherever it runs.
func TestTheProbeSweepsItsOwnLeftovers(t *testing.T) {
	dir := t.TempDir()
	orphan := filepath.Join(dir, WriteProbePrefix+"123456")
	if err := os.WriteFile(orphan, nil, 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}

	NewWriteGuard(filepath.Join(dir, "state.toml"))

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("probe leftover %s survived (stat err: %v)", orphan, err)
	}
}

// Two guards probing the same directory at once must not delete each other's
// live probe file, so only leftovers older than the grace period are swept.
func TestTheProbeLeavesALiveProbeFileAlone(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, WriteProbePrefix+"inflight")
	if err := os.WriteFile(fresh, nil, 0600); err != nil {
		t.Fatal(err)
	}

	NewWriteGuard(filepath.Join(dir, "state.toml"))

	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("a probe still in flight was swept: %v", err)
	}
}

// Nothing else in the directory is the probe's business.
func TestTheProbeSweepsNothingElse(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "peers.toml")
	if err := os.WriteFile(keep, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(keep, old, old); err != nil {
		t.Fatal(err)
	}

	NewWriteGuard(keep)

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("the sweep took a file that was not its own: %v", err)
	}
}

func TestWriteTOMLAtomicReportsReadOnly(t *testing.T) {
	skipAsRoot(t)
	dir := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	path := filepath.Join(dir, "state.toml")

	err := WriteTOMLAtomic(path, 0755, struct {
		A string `toml:"a"`
	}{A: "b"})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("error %v must carry ErrReadOnly", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q must name the path %q", err, path)
	}
}

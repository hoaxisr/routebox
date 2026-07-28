package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrReadOnly is the sentinel every RouteBox write path returns when the file it
// was asked to write cannot be written. Callers match it with errors.Is to
// answer with a state conflict (409) instead of a generic failure; the wrapped
// message names the path, because "something is read-only" is not an
// instruction and "chmod this file" is.
//
// It is deliberately one sentinel for the whole program rather than one per
// store: the panel shows a single read-only state, and the API answers 409 the
// same way whether it was config.json, users.toml or peers.toml that refused.
var ErrReadOnly = errors.New("read-only")

// ReadOnlyError wraps ErrReadOnly for a concrete path.
func ReadOnlyError(path string) error {
	return fmt.Errorf("%w: RouteBox cannot write %s (check ownership and permissions of the file and its directory)", ErrReadOnly, path)
}

// WriteProbePrefix names the throwaway file DirWritable creates. A kill between
// creation and removal would leave it behind forever, so the probe sweeps stale
// ones wherever it runs — see sweepProbeLeftovers.
const WriteProbePrefix = ".routebox-wtest-"

// probeGrace is how old a leftover probe file must be before the sweep takes it.
// Two guards can probe one directory at the same moment; without the grace
// period one would delete the other's live file.
const probeGrace = time.Minute

// DirWritable probes a directory the way an atomic write uses it: by creating
// (and immediately removing) a temp file in it. Probing beats reading the mode
// bits, which say nothing about ACLs, read-only mounts or MAC policy.
func DirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, WriteProbePrefix+"*")
	if err != nil {
		return !meansUnwritable(err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	sweepProbeLeftovers(dir)
	return true
}

// meansUnwritable reports whether a failed write says anything about
// writability. A full disk (or a hit quota) does not: nothing is locked, there
// is simply nowhere to put the bytes, and answering "read-only, check ownership
// and permissions" would send the operator looking in the wrong place. The
// write's own error says it better, so it is left to speak for itself.
func meansUnwritable(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, syscall.ENOSPC) && !errors.Is(err, syscall.EDQUOT)
}

// sweepProbeLeftovers removes probe files an earlier run was killed before
// cleaning up. Only called right after a successful probe: if the directory
// cannot be written, there is nothing to remove from it anyway. Best-effort
// throughout — a leftover we cannot delete is litter, not a failure.
func sweepProbeLeftovers(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-probeGrace)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), WriteProbePrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.Remove(filepath.Join(dir, e.Name()))
	}
}

// PathWritable reports whether RouteBox can write path.
//
// Writes go through a temp file in the same directory followed by a rename, so
// the directory must be writable as well as the file. A missing file is not a
// refusal — we will create it — and neither is a missing directory: every store
// runs MkdirAll before its first write, so what matters is whether the nearest
// existing ancestor lets us create the rest.
func PathWritable(path string) bool {
	if path == "" {
		return false
	}
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			return false
		}
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return false
		}
		f.Close()
	} else if !os.IsNotExist(err) {
		return false
	}
	return ancestorWritable(filepath.Dir(path))
}

// ancestorWritable walks up to the nearest directory that exists and probes it.
func ancestorWritable(dir string) bool {
	for {
		fi, err := os.Stat(dir)
		if err == nil {
			return fi.IsDir() && DirWritable(dir)
		}
		if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// readOnlyRecheckEvery bounds how often a standing read-only verdict is
// re-probed. The status endpoint is polled on a timer, and the probe writes to
// the filesystem — on a flash-backed router that is not free, so a broken
// install pays for at most one probe every couple of seconds and a healthy one
// pays for none at all.
const readOnlyRecheckEvery = 2 * time.Second

// WriteGuard is one file's writability: a verdict taken at startup, refreshed
// from what actually happens to writes, and re-probed while it stands negative
// so an operator who remounts rw sees the panel recover without a restart.
//
// An empty path means persistence is off (tests, and deployments without a
// state file). Such a guard never writes, so it is never read-only. A nil
// *WriteGuard behaves the same way, which is what keeps zero-value stores built
// by test literals from panicking on a save.
type WriteGuard struct {
	mu       sync.Mutex
	path     string
	readOnly bool
	probedAt time.Time
	now      func() time.Time // injected clock (tests)
}

// NewWriteGuard builds a guard for path and takes the first verdict.
func NewWriteGuard(path string) *WriteGuard {
	g := &WriteGuard{path: path, now: time.Now}
	g.mu.Lock()
	g.probeLocked()
	g.mu.Unlock()
	return g
}

// Path returns the guarded path.
func (g *WriteGuard) Path() string {
	if g == nil {
		return ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.path
}

// SetPath re-points the guard and re-takes the verdict: a different file has
// different permissions.
func (g *WriteGuard) SetPath(path string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.path = path
	g.probeLocked()
}

// IsReadOnly reports the standing verdict, re-probing a negative one at most
// every readOnlyRecheckEvery.
func (g *WriteGuard) IsReadOnly() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.readOnly && g.now().Sub(g.probedAt) >= readOnlyRecheckEvery {
		g.probeLocked()
	}
	return g.readOnly
}

// Recheck re-derives a standing read-only verdict right now, ignoring the rate
// limit IsReadOnly applies, and reports the fresh one. A writable guard still
// pays nothing: there is only something to re-derive when the verdict says no.
//
// It exists for the write paths that refuse BEFORE writing (the sing-box config
// does, because an atomic rename would otherwise sail over a file the operator
// deliberately made read-only). Such a refusal must not outlive the condition by
// even a couple of seconds: fixing the permissions and pressing Save has to work
// on that press, not on the one after. Status polls keep using IsReadOnly, whose
// rate limit is what keeps a broken install from probing the filesystem on every
// poll.
func (g *WriteGuard) Recheck() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.readOnly {
		g.probeLocked()
	}
	return g.readOnly
}

// SetReadOnly overrides the verdict. Production never calls it — the verdict is
// derived from the filesystem — it exists so tests can put a store in that state
// without chmod.
func (g *WriteGuard) SetReadOnly(readOnly bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.readOnly = readOnly
	g.probedAt = g.now()
}

// Note classifies the outcome of a write to the guarded path and is what every
// store returns from its save.
//
// A successful write is proof of writability and clears the verdict for free. A
// failed one is probed: if the path is unwritable the caller gets ErrReadOnly
// naming it (409, actionable), otherwise the original error survives untouched —
// a full disk or an encoding bug is not something chmod fixes.
//
// Classifying at the point of failure rather than guarding before the write is
// what makes "the mount went read-only while RouteBox was running" visible: a
// verdict taken at startup would still say writable.
func (g *WriteGuard) Note(err error) error {
	if g == nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.path == "" {
		return err
	}
	if err == nil {
		g.readOnly = false
		g.probedAt = g.now()
		return nil
	}
	// Already classified deeper down (WriteTOMLAtomic does its own): take its
	// word rather than probing a second time.
	if errors.Is(err, ErrReadOnly) {
		g.readOnly = true
		g.probedAt = g.now()
		return err
	}
	g.probeLocked()
	if g.readOnly {
		return ReadOnlyError(g.path)
	}
	return err
}

// probeLocked re-derives the verdict from the filesystem. Caller holds g.mu.
func (g *WriteGuard) probeLocked() {
	g.probedAt = g.now()
	if g.path == "" {
		g.readOnly = false
		return
	}
	g.readOnly = !PathWritable(g.path)
}

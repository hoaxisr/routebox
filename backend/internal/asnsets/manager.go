package asnsets

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
)

// PrefixSource is what the manager needs from RIPEstat (Fetcher implements it).
type PrefixSource interface {
	Prefixes(ctx context.Context, asn uint32) ([]netip.Prefix, error)
	Holder(ctx context.Context, asn uint32) (string, error)
}

var (
	// AllowedIntervals are the refresh periods (hours) an entry may have.
	AllowedIntervals = []int{6, 12, 24, 168}
	// ErrNotFound marks an unknown tag (API answers 404).
	ErrNotFound = errors.New("not found")
	// ErrNoPrefixes: RIPEstat answered, but no AS of the set announces
	// anything. Fetch-side like FetchError (API answers 502), unlike a failed
	// file or store write.
	ErrNoPrefixes = errors.New("no prefixes announced by any of the AS numbers")
)

const (
	// pruneGrace (seconds) keeps a freshly written set out of Prune's reach:
	// the API creates the set, releases the lock and only then adds it to the
	// config draft, so for a moment it is in no config at all.
	pruneGrace = 600
	// tickTimeout bounds one loop tick: Manager.mu is held across RIPEstat
	// calls, and with RIPEstat dead a tick must not block the API for long.
	tickTimeout = 5 * time.Minute
)

// FetchError names the AS whose prefixes could not be had.
type FetchError struct {
	ASN uint32
	Err error
}

func (e *FetchError) Error() string { return FormatASN(e.ASN) + ": " + e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// Manager owns ASN sets: their asn.toml entries and their prefix files.
//
// mu serialises every mutating operation; the Store has its own lock, which is
// always taken while mu is held, never the other way round.
type Manager struct {
	store      *Store
	dir        string
	src        PrefixSource
	now        func() int64
	mu         sync.Mutex
	lastLogged map[string]string // tag -> last logged refresh error ("" = ok)
}

// NewManager keeps prefix files for store's entries under dir (<dir>/<tag>.json).
func NewManager(store *Store, dir string, src PrefixSource) *Manager {
	return &Manager{
		store:      store,
		dir:        dir,
		src:        src,
		now:        func() int64 { return time.Now().Unix() },
		lastLogged: map[string]string{},
	}
}

// PathFor is where tag's prefix file lives (the path sing-box is pointed at).
func (m *Manager) PathFor(tag string) string { return filepath.Join(m.dir, tag+".json") }

func (m *Manager) withPath(e Entry) Entry { e.Path = m.PathFor(e.Tag); return e }

// List returns every entry, Path filled, sorted by tag.
func (m *Manager) List() []Entry {
	list := m.store.List()
	for i := range list {
		list[i] = m.withPath(list[i])
	}
	return list
}

// Get returns the entry with that tag, Path filled.
func (m *Manager) Get(tag string) (Entry, bool) {
	e, ok := m.store.Get(tag)
	if !ok {
		return Entry{}, false
	}
	return m.withPath(e), true
}

// normalize validates operator input: a safe tag, at least one AS number
// (duplicates and 0 dropped, order kept) and an allowed interval (0 → 24 h).
func normalize(tag string, asns []uint32, intervalHrs int) ([]uint32, int, error) {
	if !ValidTag(tag) {
		return nil, 0, fmt.Errorf("%w: tag %q must be 1-64 of A-Z a-z 0-9 . _ -", ErrInvalid, tag)
	}
	var uniq []uint32
	for _, a := range asns {
		if a != 0 && !slices.Contains(uniq, a) {
			uniq = append(uniq, a)
		}
	}
	if len(uniq) == 0 {
		return nil, 0, fmt.Errorf("%w: at least one AS number is required", ErrInvalid)
	}
	if intervalHrs == 0 {
		intervalHrs = 24
	}
	if !slices.Contains(AllowedIntervals, intervalHrs) {
		return nil, 0, fmt.Errorf("%w: interval must be one of %v hours", ErrInvalid, AllowedIntervals)
	}
	return uniq, intervalHrs, nil
}

// fetchAll returns the aggregated union of what every AS announces. strict: an
// AS with no prefixes is an error (create/update — usually a typo); otherwise
// only an empty union is.
func (m *Manager) fetchAll(ctx context.Context, asns []uint32, strict bool) ([]netip.Prefix, error) {
	var all []netip.Prefix
	for _, a := range asns {
		ps, err := m.src.Prefixes(ctx, a)
		if err != nil {
			return nil, &FetchError{ASN: a, Err: err}
		}
		if strict && len(ps) == 0 {
			return nil, &FetchError{ASN: a, Err: errors.New("announces no prefixes")}
		}
		all = append(all, ps...)
	}
	out := Aggregate(all)
	if len(out) == 0 {
		return nil, ErrNoPrefixes
	}
	return out, nil
}

// holders looks up names best effort: a failed lookup is an empty name.
func (m *Manager) holders(ctx context.Context, asns []uint32) map[string]string {
	h := make(map[string]string, len(asns))
	for _, a := range asns {
		name, _ := m.src.Holder(ctx, a)
		h[strconv.FormatUint(uint64(a), 10)] = name
	}
	return h
}

// Create fetches every AS, writes the prefix file and records the entry.
// Nothing is left behind on any error.
func (m *Manager) Create(ctx context.Context, tag string, asns []uint32, intervalHrs int) (Entry, error) {
	asns, intervalHrs, err := normalize(tag, asns, intervalHrs)
	if err != nil {
		return Entry{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.store.Get(tag); ok {
		return Entry{}, fmt.Errorf("%w: ASN set %q already exists", ErrInvalid, tag)
	}
	prefixes, err := m.fetchAll(ctx, asns, true)
	if err != nil {
		return Entry{}, err
	}
	holders := m.holders(ctx, asns) // before the write: nothing slow between file and entry
	path := m.PathFor(tag)
	if err := WritePrefixFile(path, prefixes); err != nil {
		return Entry{}, err
	}
	e := Entry{
		Tag:         tag,
		ASNs:        asns,
		Holders:     holders,
		IntervalHrs: intervalHrs,
		UpdatedAt:   m.now(),
		PrefixCount: len(prefixes),
	}
	if err := m.store.Put(e); err != nil {
		os.Remove(path) // no entry, no file
		return Entry{}, err
	}
	return m.withPath(e), nil
}

// Update replaces the AS list and interval of an existing set, refetching
// everything; the tag and path stay. A fetch or file-write error leaves both
// file and entry as they were. Holders are looked up before the file is
// written so that nothing slow sits between the write and store.Put; should
// Put still fail, the file already holds the new prefixes beside the old entry
// until the next successful Refresh of that entry rewrites it.
func (m *Manager) Update(ctx context.Context, tag string, asns []uint32, intervalHrs int) (Entry, error) {
	asns, intervalHrs, err := normalize(tag, asns, intervalHrs)
	if err != nil {
		return Entry{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.store.Get(tag)
	if !ok {
		return Entry{}, fmt.Errorf("ASN set %q: %w", tag, ErrNotFound)
	}
	prefixes, err := m.fetchAll(ctx, asns, true)
	if err != nil {
		return Entry{}, err
	}
	holders := m.holders(ctx, asns)
	if err := WritePrefixFile(m.PathFor(tag), prefixes); err != nil {
		return Entry{}, err
	}
	e.ASNs, e.Holders, e.IntervalHrs = asns, holders, intervalHrs
	e.UpdatedAt, e.PrefixCount, e.LastError = m.now(), len(prefixes), ""
	if err := m.store.Put(e); err != nil {
		return Entry{}, err
	}
	return m.withPath(e), nil
}

// Refresh refetches a set's prefixes. On failure (or an empty union) the old
// file stays in service and the error is recorded in LastError; the entry is
// returned alongside the error so callers can show it.
func (m *Manager) Refresh(ctx context.Context, tag string) (Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refreshLocked(ctx, tag)
}

func (m *Manager) refreshLocked(ctx context.Context, tag string) (Entry, error) {
	e, ok := m.store.Get(tag)
	if !ok {
		return Entry{}, fmt.Errorf("ASN set %q: %w", tag, ErrNotFound)
	}
	prefixes, err := m.fetchAll(ctx, e.ASNs, false)
	if err == nil {
		err = WritePrefixFile(m.PathFor(tag), prefixes)
	}
	if err != nil {
		e.LastError = err.Error()
		_ = m.store.Put(e) // best effort; the old file stays in service either way
		return m.withPath(e), err
	}
	e.UpdatedAt, e.PrefixCount, e.LastError = m.now(), len(prefixes), ""
	if perr := m.store.Put(e); perr != nil {
		return m.withPath(e), perr
	}
	return m.withPath(e), nil
}

// Delete removes the entry and its file. Config references are the caller's
// business (the API only deletes sets no config mentions; Prune does the rest).
func (m *Manager) Delete(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteLocked(tag)
}

func (m *Manager) deleteLocked(tag string) error {
	if err := m.store.Delete(tag); err != nil {
		return err
	}
	if err := os.Remove(m.PathFor(tag)); err != nil && !os.IsNotExist(err) {
		return err
	}
	delete(m.lastLogged, tag)
	return nil
}

// Prune removes sets no config references any more. inConfig must answer for
// the active AND the working config: a set deleted in the draft is still in
// service until Apply, and its file must outlive that. Sets written less than
// pruneGrace ago are left alone: the API adds a new set to the draft only
// after Create has returned, and that gap must not look like an orphan.
//
// inConfig runs under Manager.mu: it must not call back into Manager, and
// must not take a lock whose holder ever does (the boot wiring only reads
// config.Manager, which never calls asnsets).
func (m *Manager) Prune(inConfig func(tag string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, e := range m.store.List() {
		if now-e.UpdatedAt < pruneGrace || inConfig(e.Tag) {
			continue
		}
		if err := m.deleteLocked(e.Tag); err != nil {
			log.Printf("asnsets: %s: prune: %v", e.Tag, err)
		}
	}
}

// RestoreMissing refetches every set whose prefix file is gone. If the fetch
// fails it writes an EMPTY rule set instead: a missing `local` rule-set file is
// FATAL at sing-box start, an empty one matches nothing until the next
// successful refresh. Meant to run synchronously at boot, before amnezia-box
// autostart, with a bounded context.
func (m *Manager) RestoreMissing(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.store.List() {
		path := m.PathFor(e.Tag)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			continue
		}
		_, err := m.refreshLocked(ctx, e.Tag)
		if err == nil {
			continue
		}
		// refreshLocked may have written the file and only failed on store.Put;
		// never clobber good prefixes with a placeholder.
		if _, serr := os.Stat(path); serr == nil {
			log.Printf("asnsets: %s: prefix file restored but entry could not be saved: %v", e.Tag, err)
			continue
		}
		if werr := WritePrefixFile(path, nil); werr != nil {
			log.Printf("asnsets: %s: prefix file missing, refetch failed (%v) and placeholder could not be written: %v", e.Tag, err, werr)
			continue
		}
		// UpdatedAt = 0 makes the set due on the very next tick instead of
		// after a full interval of matching nothing.
		if cur, ok := m.store.Get(e.Tag); ok {
			cur.UpdatedAt = 0
			_ = m.store.Put(cur) // best effort, like LastError above
		}
		log.Printf("asnsets: %s: prefix file missing and refetch failed; empty placeholder written, retry on next tick: %v", e.Tag, err)
	}
}

// RefreshDue refreshes every set older than its interval. Failures are logged
// once per distinct error text, and recovery once, so a flapping RIPEstat does
// not flood the log.
func (m *Manager) RefreshDue(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, e := range m.store.List() {
		if now-e.UpdatedAt < int64(e.IntervalHrs)*3600 {
			continue
		}
		_, err := m.refreshLocked(ctx, e.Tag)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if msg == m.lastLogged[e.Tag] {
			continue
		}
		if err != nil {
			log.Printf("asnsets: %s: refresh failed: %v", e.Tag, err)
		} else {
			log.Printf("asnsets: %s: refresh ok again", e.Tag)
		}
		m.lastLogged[e.Tag] = msg
	}
}

// RunLoop prunes and refreshes every interval until stop is closed. Each
// tick's refresh is bounded by tickTimeout so a dead RIPEstat cannot hold
// Manager.mu, and with it the API, indefinitely.
func (m *Manager) RunLoop(interval time.Duration, inConfig func(string) bool, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			m.Prune(inConfig)
			m.tick()
		}
	}
}

func (m *Manager) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), tickTimeout)
	defer cancel()
	m.RefreshDue(ctx)
}

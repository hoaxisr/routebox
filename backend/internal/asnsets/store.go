package asnsets

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/BurntSushi/toml"

	"routebox/backend/internal/util"
)

// Entry is one ASN set's metadata; the prefixes themselves live in its file.
type Entry struct {
	Tag         string            `toml:"tag" json:"tag"`
	ASNs        []uint32          `toml:"asns" json:"asns"`
	Holders     map[string]string `toml:"holders" json:"holders"` // key: decimal ASN
	IntervalHrs int               `toml:"interval_hrs" json:"interval_hrs"`
	UpdatedAt   int64             `toml:"updated_at" json:"updated_at"`
	PrefixCount int               `toml:"prefix_count" json:"prefix_count"`
	LastError   string            `toml:"last_error" json:"last_error"`
	Path        string            `toml:"-" json:"path"` // filled by Manager
}

// Store persists entries in asn.toml beside settings.toml.
type Store struct {
	path  string
	mu    sync.Mutex
	byTag map[string]Entry
	guard *util.WriteGuard
}

// NewStore builds a store for path and takes the first writability verdict.
// An empty path keeps everything in memory (tests).
func NewStore(path string) *Store {
	return &Store{path: path, byTag: map[string]Entry{}, guard: util.NewWriteGuard(path)}
}

// Load reads the file; a missing one is an empty store, not an error.
//
// A file that does not parse, or carries an unsafe or duplicate tag (hand
// edits), is refused as a whole and the in-memory entries are left as they
// were: the caller must then keep the manager out of service, because the
// first Put would rewrite asn.toml from whatever is in memory and every other
// set's metadata would be gone. Tags become file names (PathFor), so an
// invalid one must never get in through the file.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc struct {
		Sets []Entry `toml:"sets"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", s.path, err)
	}
	byTag := make(map[string]Entry, len(doc.Sets))
	for _, e := range doc.Sets {
		if !ValidTag(e.Tag) {
			return fmt.Errorf("parse %s: invalid tag %q", s.path, e.Tag)
		}
		if _, dup := byTag[e.Tag]; dup {
			return fmt.Errorf("parse %s: duplicate tag %q", s.path, e.Tag)
		}
		byTag[e.Tag] = e
	}
	s.byTag = byTag
	return nil
}

// List returns deep copies of every entry, sorted by tag.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Store) listLocked() []Entry {
	out := make([]Entry, 0, len(s.byTag))
	for _, e := range s.byTag {
		out = append(out, clone(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
}

// Get returns a deep copy of the entry with that tag.
func (s *Store) Get(tag string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byTag[tag]
	return clone(e), ok
}

// Put inserts or replaces an entry and persists. On a failed write the
// in-memory map is rolled back so memory and disk agree.
func (s *Store) Put(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.byTag[e.Tag]
	s.byTag[e.Tag] = clone(e)
	if err := s.saveLocked(); err != nil {
		if had {
			s.byTag[e.Tag] = prev
		} else {
			delete(s.byTag, e.Tag)
		}
		return err
	}
	return nil
}

// Delete removes the entry with that tag (a missing one is not an error) and
// persists, rolling back on a failed write.
func (s *Store) Delete(tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.byTag[tag]
	if !had {
		return nil
	}
	delete(s.byTag, tag)
	if err := s.saveLocked(); err != nil {
		s.byTag[tag] = prev
		return err
	}
	return nil
}

// saveLocked persists the set. A failure that is about writability comes back
// as util.ErrReadOnly naming the file (409). Caller holds s.mu.
func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	return s.guard.Note(writeAtomic(s.path, func(f *os.File) error {
		return toml.NewEncoder(f).Encode(struct {
			Sets []Entry `toml:"sets"`
		}{s.listLocked()})
	}))
}

// WritePrefixFile writes a sing-box source rule set with one ip_cidr rule,
// atomically: sing-box watches this file and must never see half of it.
//
// len(prefixes)==0 writes {"version":2,"rules":[]} — a rule set that matches
// nothing. Verified with the fork binary (1.15.0-alpha.9-awgm.30): `check` and
// `run` accept it, whereas an empty ip_cidr list inside a rule is not used.
//
// An unwritable path comes back wrapping util.ErrReadOnly, as every other
// RouteBox write does.
func WritePrefixFile(path string, prefixes []netip.Prefix) error {
	rules := []any{}
	if len(prefixes) > 0 {
		cidrs := make([]string, len(prefixes))
		for i, p := range prefixes {
			cidrs[i] = p.String()
		}
		rules = append(rules, map[string]any{"ip_cidr": cidrs})
	}
	doc := map[string]any{"version": 2, "rules": rules}
	return util.ClassifyWriteErr(path, writeAtomic(path, func(f *os.File) error { return json.NewEncoder(f).Encode(doc) }))
}

// writeAtomic creates the parent directory, writes a sibling temp file, fsyncs
// it and renames it over path. Readers (sing-box's file watcher included) see
// either the old file or the whole new one; a failure leaves path untouched.
func writeAtomic(path string, write func(*os.File) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0644); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// clone deep-copies the slices and maps so callers can mutate what they get
// without reaching into the store.
func clone(e Entry) Entry {
	e.ASNs = append([]uint32(nil), e.ASNs...)
	h := make(map[string]string, len(e.Holders))
	for k, v := range e.Holders {
		h[k] = v
	}
	e.Holders = h
	return e
}

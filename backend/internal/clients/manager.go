package clients

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	"routebox/backend/internal/util"
)

// Entry represents one known LAN client.
type Entry struct {
	IP        string `toml:"ip" json:"ip"`
	Name      string `toml:"name" json:"name"`
	Note      string `toml:"note" json:"note"`
	FirstSeen int64  `toml:"first_seen" json:"first_seen"`
	LastSeen  int64  `toml:"last_seen" json:"last_seen"`
}

// Manager owns the in-memory index of clients and TOML persistence.
type Manager struct {
	path  string
	mu    sync.RWMutex
	byIP  map[string]*Entry
	dirty bool
	guard *util.WriteGuard
}

// New constructs a Manager. Pass empty path to disable persistence (used by tests).
func New(path string) *Manager {
	return &Manager{path: path, byIP: map[string]*Entry{}, guard: util.NewWriteGuard(path)}
}

// GetPath returns the file this store persists to ("" when persistence is off).
func (m *Manager) GetPath() string { return m.path }

// IsReadOnly reports whether the store's file cannot be written, so the panel
// can show one read-only state for every file RouteBox persists.
func (m *Manager) IsReadOnly() bool { return m.guard.IsReadOnly() }

// Load reads the TOML file if path is set and file exists. Must be called once
// at startup before any Observe/SetName calls — Load replaces the in-memory map
// wholesale, so any IPs already observed since New(...) would be lost.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.path == "" {
		return nil
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var doc struct {
		Clients []Entry `toml:"clients"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", m.path, err)
	}
	m.byIP = make(map[string]*Entry, len(doc.Clients))
	for i := range doc.Clients {
		e := doc.Clients[i]
		m.byIP[e.IP] = &e
	}
	return nil
}

// Save writes the TOML file (no-op if no path or not dirty). A failure that is
// about writability comes back as util.ErrReadOnly naming the file, so the API
// answers 409 with something the operator can act on instead of a raw 500.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.path == "" || !m.dirty {
		return nil
	}
	if err := m.guard.Note(m.writeLocked()); err != nil {
		return err
	}
	m.dirty = false
	return nil
}

// writeLocked renders and writes the file. Caller holds m.mu.
func (m *Manager) writeLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	doc := struct {
		Clients []Entry `toml:"clients"`
	}{Clients: m.listLocked()}
	if err := toml.NewEncoder(f).Encode(doc); err != nil {
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
	return os.Rename(tmp, m.path)
}

// Observe is called whenever an IP is seen in traffic. Creates or updates the entry.
func (m *Manager) Observe(ip string, when time.Time) {
	if ip == "" {
		return
	}
	ts := when.Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byIP[ip]
	if !ok {
		m.byIP[ip] = &Entry{IP: ip, FirstSeen: ts, LastSeen: ts}
		m.dirty = true
		return
	}
	if e.LastSeen != ts {
		e.LastSeen = ts
		m.dirty = true
	}
}

// SetName updates the friendly name and note for an IP, creating an entry if needed.
func (m *Manager) SetName(ip, name, note string) error {
	if ip == "" {
		return fmt.Errorf("ip is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byIP[ip]
	if !ok {
		now := time.Now().Unix()
		e = &Entry{IP: ip, FirstSeen: now, LastSeen: now}
		m.byIP[ip] = e
	}
	e.Name = name
	e.Note = note
	m.dirty = true
	return nil
}

// Forget removes an entry. The IP can return on next sighting (without name).
func (m *Manager) Forget(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byIP[ip]; ok {
		delete(m.byIP, ip)
		m.dirty = true
	}
}

// Get returns a copy of the entry.
func (m *Manager) Get(ip string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.byIP[ip]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

// List returns all entries sorted by last_seen descending.
func (m *Manager) List() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []Entry {
	out := make([]Entry, 0, len(m.byIP))
	for _, e := range m.byIP {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

// StartPersistLoop periodically saves dirty state; on stop it performs a
// final flush and closes done.
func (m *Manager) StartPersistLoop(interval time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	if m.path == "" {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.Save(); err != nil {
				log.Printf("clients: save failed: %v", err)
			}
		case <-stop:
			if err := m.Save(); err != nil {
				log.Printf("clients: final save failed: %v", err)
			}
			return
		}
	}
}

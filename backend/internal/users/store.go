package users

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// PanelUser is a first-class panel user with a stable ID. Bindings link the user
// to one or more server inbounds. Enabled/ExpiresAt/Token are stored but NOT
// enforced in Phase 2 (Phases 3-4).
type PanelUser struct {
	ID        string `toml:"id" json:"id"`
	Name      string `toml:"name" json:"name"`
	Enabled   bool   `toml:"enabled" json:"enabled"`
	ExpiresAt int64  `toml:"expires_at" json:"expires_at"` // unix; 0 = never
	Token     string `toml:"token" json:"token"`           // Phase 3
	// TokenDisabled marks a deliberately-revoked subscription. While true, the
	// reconciler never auto-re-mints a token for this user (sticky revoke); a
	// Rotate clears it (re-enables). Distinct from Enabled (Phase 4 lifecycle).
	TokenDisabled bool      `toml:"token_disabled" json:"token_disabled"` // Phase 3
	Bindings      []Binding `toml:"bindings" json:"bindings"`
}

// Binding ties a PanelUser to a credential inside one server inbound. Name/Flow
// are cached display fields for the UI.
type Binding struct {
	InboundTag string `toml:"inbound_tag" json:"inbound_tag"`
	Credential string `toml:"credential" json:"credential"`
	Protocol   string `toml:"protocol" json:"protocol"`
	Name       string `toml:"name" json:"name"`
	Flow       string `toml:"flow" json:"flow"`
}

// Manager owns the panel-user registry and its TOML persistence.
type Manager struct {
	path string
	mu   sync.RWMutex
	byID map[string]*PanelUser
}

// NewManager constructs a Manager. Empty path disables persistence (tests).
func NewManager(path string) *Manager {
	return &Manager{path: path, byID: map[string]*PanelUser{}}
}

// Load reads the TOML file if present, replacing the in-memory set. A missing
// file is not an error (empty registry).
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
		Users []PanelUser `toml:"users"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", m.path, err)
	}
	m.byID = make(map[string]*PanelUser, len(doc.Users))
	for i := range doc.Users {
		u := doc.Users[i]
		if u.ID == "" {
			log.Printf("users: skipping registry entry with empty id (name=%q)", u.Name)
			continue
		}
		if _, dup := m.byID[u.ID]; dup {
			log.Printf("users: skipping duplicate registry id %q", u.ID)
			continue
		}
		m.byID[u.ID] = &u
	}
	return nil
}

// saveLocked writes the registry atomically with 0600 perms. Caller holds m.mu.
func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	doc := struct {
		Users []PanelUser `toml:"users"`
	}{Users: m.listLocked()}
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

// Put inserts or replaces a user (storing a deep copy) and persists. If
// persistence fails the in-memory state is rolled back to its prior value so
// memory never diverges from disk.
func (m *Manager) Put(u *PanelUser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev, existed := m.byID[u.ID]
	m.byID[u.ID] = cloneUser(u)
	if err := m.saveLocked(); err != nil {
		if existed {
			m.byID[u.ID] = prev
		} else {
			delete(m.byID, u.ID)
		}
		return err
	}
	return nil
}

// Delete removes a user and persists. No-op if absent. If persistence fails the
// removed entry is restored so memory never diverges from disk.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev, existed := m.byID[id]
	if !existed {
		return nil
	}
	delete(m.byID, id)
	if err := m.saveLocked(); err != nil {
		m.byID[id] = prev
		return err
	}
	return nil
}

// Get returns a deep copy of the user with the given ID.
func (m *Manager) Get(id string) (PanelUser, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.byID[id]
	if !ok {
		return PanelUser{}, false
	}
	return *cloneUser(u), true
}

// List returns deep copies of all users sorted by name then ID.
func (m *Manager) List() []PanelUser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked()
}

// listLocked returns deep value copies sorted by name then ID. Caller holds m.mu.
func (m *Manager) listLocked() []PanelUser {
	out := make([]PanelUser, 0, len(m.byID))
	for _, u := range m.byID {
		out = append(out, *cloneUser(u))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// cloneUser deep-copies a user, including a fresh Bindings backing array, so the
// caller can never mutate the registry's internal state through the copy.
func cloneUser(u *PanelUser) *PanelUser {
	cp := *u
	if u.Bindings != nil {
		cp.Bindings = make([]Binding, len(u.Bindings))
		copy(cp.Bindings, u.Bindings)
	}
	return &cp
}

// newID returns a short, URL-safe, lowercase random id (crypto/rand, 10 chars).
// Used by the reconciler (Task 2) and the API (Task 5); defined here so both
// share one generator.
func newID() (string, error) {
	b := make([]byte, 7) // 7 bytes -> 56 bits -> >= 11 base32 chars; trim to 10
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return strings.ToLower(enc[:10]), nil
}

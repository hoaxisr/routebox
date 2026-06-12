package subscriptions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Subscription is one configured subscription URL and its last-refresh result.
type Subscription struct {
	ID          string `toml:"id" json:"id"`
	Name        string `toml:"name" json:"name"`
	URL         string `toml:"url" json:"url"`
	IntervalHrs int    `toml:"interval_hrs" json:"interval_hrs"`
	LastUpdated int64  `toml:"last_updated" json:"last_updated"`
	LastError   string `toml:"last_error" json:"last_error"`
	NodeCount   int    `toml:"node_count" json:"node_count"`
}

// Manager owns the subscription set and TOML persistence. CRUD persists
// synchronously (changes are user-initiated and rare).
type Manager struct {
	path string
	mu   sync.RWMutex
	byID map[string]*Subscription
}

// NewManager constructs a Manager. Empty path disables persistence (tests).
func NewManager(path string) *Manager {
	return &Manager{path: path, byID: map[string]*Subscription{}}
}

// Load reads the TOML file if present, replacing the in-memory set.
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
		Subscriptions []Subscription `toml:"subscriptions"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", m.path, err)
	}
	m.byID = make(map[string]*Subscription, len(doc.Subscriptions))
	for i := range doc.Subscriptions {
		s := doc.Subscriptions[i]
		m.byID[s.ID] = &s
	}
	return nil
}

func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	doc := struct {
		Subscriptions []Subscription `toml:"subscriptions"`
	}{Subscriptions: m.listLocked()}
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

// slugify converts a name to a lowercase URL-safe ID. Runs of non-alphanumeric
// characters are collapsed to a single hyphen between multi-char alnum tokens;
// single-character tokens are concatenated without a separator. This produces
// "home-vpn" from "Home VPN" and "vpn" from both "VPN" and "v p n", making
// slug-collision detection reliable.
func slugify(name string) string {
	// Split into alnum tokens.
	lower := strings.ToLower(strings.TrimSpace(name))
	var tokens []string
	var cur strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	if len(tokens) == 0 {
		return ""
	}
	// Join: insert a hyphen between consecutive tokens only when both are
	// multi-character; single-char tokens are concatenated directly.
	var b strings.Builder
	b.WriteString(tokens[0])
	for i := 1; i < len(tokens); i++ {
		if len(tokens[i-1]) > 1 && len(tokens[i]) > 1 {
			b.WriteByte('-')
		}
		b.WriteString(tokens[i])
	}
	return b.String()
}

// Add creates a subscription. Errors on empty name or slug collision.
func (m *Manager) Add(name, url string, intervalHrs int) (Subscription, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Subscription{}, fmt.Errorf("name is required")
	}
	id := slugify(name)
	if id == "" {
		return Subscription{}, fmt.Errorf("name must contain at least one alphanumeric character")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.byID[id]; exists {
		return Subscription{}, fmt.Errorf("subscription %q already exists", name)
	}
	for _, s := range m.byID {
		if s.Name == name {
			return Subscription{}, fmt.Errorf("subscription %q already exists", name)
		}
	}
	s := &Subscription{ID: id, Name: name, URL: url, IntervalHrs: intervalHrs}
	m.byID[id] = s
	if err := m.saveLocked(); err != nil {
		delete(m.byID, id)
		return Subscription{}, err
	}
	return *s, nil
}

// Update changes URL and interval; Name and ID are immutable.
func (m *Manager) Update(id, url string, intervalHrs int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok {
		return fmt.Errorf("subscription %q not found", id)
	}
	s.URL = url
	s.IntervalHrs = intervalHrs
	return m.saveLocked()
}

// Delete removes a subscription.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[id]; !ok {
		return fmt.Errorf("subscription %q not found", id)
	}
	delete(m.byID, id)
	return m.saveLocked()
}

// SetResult records a refresh outcome. Success (lastErr=="") bumps LastUpdated +
// NodeCount; error records LastError but keeps prior NodeCount/LastUpdated.
func (m *Manager) SetResult(id string, nodeCount int, lastErr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok {
		return fmt.Errorf("subscription %q not found", id)
	}
	s.LastError = lastErr
	if lastErr == "" {
		s.NodeCount = nodeCount
		s.LastUpdated = time.Now().Unix()
	}
	return m.saveLocked()
}

// Get returns a copy.
func (m *Manager) Get(id string) (Subscription, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byID[id]
	if !ok {
		return Subscription{}, false
	}
	return *s, true
}

// List returns all subscriptions sorted by name.
func (m *Manager) List() []Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []Subscription {
	out := make([]Subscription, 0, len(m.byID))
	for _, s := range m.byID {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

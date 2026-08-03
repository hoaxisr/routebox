// Package mtproto runs RouteBox's built-in Telegram MTProto proxy and owns the
// client roster it serves.
//
// It is shaped like internal/awg rather than internal/users: its credentials
// live in their own file behind their own page, and never reach config.json.
// An MTProto client is not a sing-box inbound user, so it is not one here
// either.
package mtproto

import (
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	"routebox/backend/internal/util"
)

// ErrBlankName rejects the one field that cannot be defaulted: the store key.
var ErrBlankName = errors.New("client name must not be blank")

// Client is one issued MTProto credential.
//
// Secret is the 32-hex-character key half of the tg:// secret. The masking
// domain is deliberately not stored per client: it is panel-wide, and baking a
// copy into every row would let the two drift apart.
type Client struct {
	Name      string `toml:"name"`
	Secret    string `toml:"secret"`
	Enabled   bool   `toml:"enabled"`
	CreatedAt int64  `toml:"created_at"`
	ExpiresAt int64  `toml:"expires_at"` // unix sec; 0 = never
}

// file is the on-disk shape. Clients are an array rather than a table keyed by
// name so that names containing dots or spaces need no escaping.
type file struct {
	Clients []Client `toml:"clients"`
}

// Store persists clients to mtproto.toml, atomically and 0600 in a 0700
// directory, mirroring awg/store.go. Keyed by Name.
type Store struct {
	path   string
	mu     sync.RWMutex
	byName map[string]*Client
	guard  *util.WriteGuard
}

// NewStore constructs a Store. An empty path disables persistence (tests).
func NewStore(path string) *Store {
	return &Store{
		path:   path,
		byName: map[string]*Client{},
		guard:  util.NewWriteGuard(path),
	}
}

// GetPath returns the file this store persists to ("" when persistence is off).
func (s *Store) GetPath() string { return s.path }

// IsReadOnly reports whether the file cannot be written, so the panel can show
// one read-only state across everything RouteBox persists.
func (s *Store) IsReadOnly() bool { return s.guard.IsReadOnly() }

// Load reads the file if present, replacing the in-memory roster.
//
// A missing file is not an error — that is a fresh install. A malformed one is:
// starting with an empty roster would quietly revoke every client at once.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		return nil
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	var doc file
	if err := toml.Unmarshal(data, &doc); err != nil {
		return err
	}

	byName := make(map[string]*Client, len(doc.Clients))

	for i := range doc.Clients {
		c := doc.Clients[i]
		byName[c.Name] = &c
	}

	s.byName = byName

	return nil
}

// saveLocked writes atomically via the shared primitive. Caller holds s.mu.
func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}

	// dir 0700: this directory holds client secrets, same as the AWG peer keys.
	return s.guard.Note(util.WriteTOMLAtomic(s.path, 0700, file{Clients: s.listLocked()}))
}

// Put inserts or replaces a client by name.
//
// The in-memory map is rolled back when the write fails, so a read-only disk
// cannot leave the roster claiming a client the proxy would lose on restart.
func (s *Store) Put(c Client) error {
	if c.Name == "" {
		return ErrBlankName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, existed := s.byName[c.Name]
	s.byName[c.Name] = &c

	if err := s.saveLocked(); err != nil {
		if existed {
			s.byName[c.Name] = prev
		} else {
			delete(s.byName, c.Name)
		}

		return err
	}

	return nil
}

// Delete removes a client. Deleting an unknown name is not an error: the caller
// wanted it gone, and it is.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, existed := s.byName[name]
	if !existed {
		return nil
	}

	delete(s.byName, name)

	if err := s.saveLocked(); err != nil {
		s.byName[name] = prev

		return err
	}

	return nil
}

// Get returns a copy of one client.
func (s *Store) Get(name string) (Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.byName[name]
	if !ok {
		return Client{}, false
	}

	return *c, true
}

// List returns every client, sorted by name so the roster order is stable
// between polls.
func (s *Store) List() []Client {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listLocked()
}

func (s *Store) listLocked() []Client {
	out := make([]Client, 0, len(s.byName))

	for _, c := range s.byName {
		out = append(out, *c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// Active returns the clients the proxy should currently accept: enabled, and
// not yet at their expiry. Expiry is inclusive — at the deadline the client is
// already out.
func (s *Store) Active(now time.Time) []Client {
	out := make([]Client, 0)

	for _, c := range s.List() {
		if !c.Enabled {
			continue
		}

		if c.ExpiresAt != 0 && now.Unix() >= c.ExpiresAt {
			continue
		}

		out = append(out, c)
	}

	return out
}

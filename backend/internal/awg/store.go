package awg

import (
	"os"
	"sort"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	"routebox/backend/internal/util"
)

// Peer is one client's stored secret material. The [Peer] in awg-rb0.conf holds
// only the PUBLIC half; this sidecar holds the secrets needed to re-render .conf/QR.
type Peer struct {
	PublicKey    string `toml:"public_key"`
	PrivateKey   string `toml:"private_key"`
	PresharedKey string `toml:"preshared_key"`
	Address      string `toml:"address"` // "<ip>/32"
	Name         string `toml:"name"`
	CreatedAt    int64  `toml:"created_at"`
	ExpiresAt    int64  `toml:"expires_at"` // unix sec; 0 = never expires
}

// Store persists peer secrets to peers.toml (0600, dir 0700), atomically, mirroring
// users/store.go. Keyed by PublicKey.
type Store struct {
	path      string
	mu        sync.RWMutex
	byPK      map[string]*Peer
	serverKey string       // server private key (singbox backend); persisted top-level
	headerKey string       // AWG3 header-protection key (singbox backend); persisted top-level
	ulaPrefix string       // IPv6-broker ULA prefix (singbox backend); persisted top-level
	now       func() int64 // injected clock
	guard     *util.WriteGuard
}

// NewStore constructs a Store. Empty path disables persistence (tests).
func NewStore(path string) *Store {
	return &Store{path: path, byPK: map[string]*Peer{}, now: func() int64 { return time.Now().Unix() }, guard: util.NewWriteGuard(path)}
}

// GetPath returns the file this store persists to ("" when persistence is off).
func (s *Store) GetPath() string { return s.path }

// IsReadOnly reports whether peers.toml cannot be written. It lives in its own
// directory, so it can be read-only while the sing-box config is not — the panel
// shows one read-only state covering both.
func (s *Store) IsReadOnly() bool { return s.guard.IsReadOnly() }

// Load reads peers.toml if present (missing file is not an error).
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var doc struct {
		ServerKey string `toml:"server_key"`
		HeaderKey string `toml:"header_key"`
		ULAPrefix string `toml:"ula_prefix"`
		Peers     []Peer `toml:"peers"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return err
	}
	s.serverKey = doc.ServerKey
	s.headerKey = doc.HeaderKey
	s.ulaPrefix = doc.ULAPrefix
	s.byPK = make(map[string]*Peer, len(doc.Peers))
	for i := range doc.Peers {
		p := doc.Peers[i]
		if p.PublicKey != "" {
			s.byPK[p.PublicKey] = &p
		}
	}
	return nil
}

// saveLocked writes atomically with 0600 (dir 0700) via the shared
// util.WriteTOMLAtomic primitive. Caller holds s.mu.
func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	// header_key is omitempty so the kernel backend's peers.toml (which never sets
	// it) stays byte-identical to the pre-awg3 format.
	doc := struct {
		ServerKey string `toml:"server_key,omitempty"`
		HeaderKey string `toml:"header_key,omitempty"`
		ULAPrefix string `toml:"ula_prefix,omitempty"`
		Peers     []Peer `toml:"peers"`
	}{ServerKey: s.serverKey, HeaderKey: s.headerKey, ULAPrefix: s.ulaPrefix, Peers: s.listLocked()}
	// dir 0700: this directory holds client private keys, stricter than the
	// 0755 users/store.go uses for its registry.
	return s.guard.Note(util.WriteTOMLAtomic(s.path, 0700, doc))
}

// ServerKey returns the persisted server private key ("" if none).
func (s *Store) ServerKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serverKey
}

// SetServerKey persists the server private key alongside the peers.
func (s *Store) SetServerKey(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverKey = k
	return s.saveLocked()
}

// HeaderKey returns the persisted AWG3 header-protection key ("" if none).
func (s *Store) HeaderKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.headerKey
}

// SetHeaderKey persists the AWG3 header-protection key alongside the peers.
func (s *Store) SetHeaderKey(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headerKey = k
	return s.saveLocked()
}

// ULAPrefix returns the persisted IPv6-broker ULA prefix ("" if none).
func (s *Store) ULAPrefix() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ulaPrefix
}

// SetULAPrefix persists the ULA prefix alongside the peers (write-once by caller).
func (s *Store) SetULAPrefix(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ulaPrefix = p
	return s.saveLocked()
}

// Put inserts/replaces a peer (stamping CreatedAt if unset) and persists, rolling
// back in-memory state on save failure.
func (s *Store) Put(p Peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.CreatedAt == 0 {
		p.CreatedAt = s.now()
	}
	prev, existed := s.byPK[p.PublicKey]
	cp := p
	s.byPK[p.PublicKey] = &cp
	if err := s.saveLocked(); err != nil {
		if existed {
			s.byPK[p.PublicKey] = prev
		} else {
			delete(s.byPK, p.PublicKey)
		}
		return err
	}
	return nil
}

// Delete removes a peer secret and persists (rollback on failure).
func (s *Store) Delete(pk string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, existed := s.byPK[pk]
	if !existed {
		return nil
	}
	delete(s.byPK, pk)
	if err := s.saveLocked(); err != nil {
		s.byPK[pk] = prev
		return err
	}
	return nil
}

// Get returns a copy of the peer secret.
func (s *Store) Get(pk string) (Peer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.byPK[pk]
	if !ok {
		return Peer{}, false
	}
	return *p, true
}

// List returns value copies of all stored peers sorted by PublicKey (lock-safe).
func (s *Store) List() []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listLocked()
}

// listLocked returns value copies of all stored peers sorted by PublicKey. Caller
// holds s.mu.
func (s *Store) listLocked() []Peer {
	out := make([]Peer, 0, len(s.byPK))
	for _, p := range s.byPK {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublicKey < out[j].PublicKey })
	return out
}

// Reconcile reports which peers of the LIVE interface are not ours. "Ours" is
// the store, whole: a suspended (expired) peer is intentionally absent from the
// conf (peerLines skips it so Enable/Apply cannot resurrect it) yet stays in the
// store, and its secrets MUST survive reconcile — otherwise RenewPeer becomes
// impossible for every expired client.
//
// It used to take the conf's pubkeys as well and treat "ours" as the union. That
// was never a second source: the conf is RENDERED from the store
// (RenderServer(sc, peerLines())), and the caller fed peerLines() straight back
// in, so the argument was always a subset of the keys below. Its only effect was
// to suggest that a hand-edited .conf could vouch for a peer the store has never
// heard of — which cost one reviewer's afternoon and a corrected test comment.
// A conf-only peer cannot survive an Enable anyway; if one ever needs to be
// tolerated, that is a decision to make deliberately, not to inherit from a
// parameter nobody was passing.
//
// The store is never mutated: reconcile only tells the caller what to remove from
// the interface (leftovers of a crash or of manual `awg set` edits). livePubs is
// produced by parseShowPeers — the store stays Runner-free.
func (s *Store) Reconcile(livePubs []string) (removeLive []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ours := make(map[string]bool, len(s.byPK))
	for pk := range s.byPK {
		ours[pk] = true
	}
	for _, lp := range livePubs {
		if !ours[lp] {
			removeLive = append(removeLive, lp)
		}
	}
	sort.Strings(removeLive) // deterministic order for the caller's removal log
	return removeLive
}

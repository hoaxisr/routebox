package mtproto

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/9seconds/mtg/v2/antireplay"
	"github.com/9seconds/mtg/v2/ipblocklist"
	"github.com/9seconds/mtg/v2/mtglib"
	network "github.com/9seconds/mtg/v2/network/v2"
)

var (
	// ErrNoActiveClients is returned when a start would leave the proxy with an
	// empty secret list. mtglib refuses that, and a proxy nobody can
	// authenticate against is not worth a listener.
	ErrNoActiveClients = errors.New("no enabled, unexpired clients")

	// ErrNoMaskingDomain is a distinct error because it is the one setting an
	// operator has to supply by hand. Without it mtglib.Secret.Valid() fails
	// and the operator sees a bare "secret is invalid" naming no field.
	ErrNoMaskingDomain = errors.New("masking domain is not configured")

	// ErrAlreadyRunning guards against leaking the current listener.
	ErrAlreadyRunning = errors.New("proxy is already running")
)

// allowAll is the IP allowlist RouteBox hands mtglib.
//
// ipblocklist.NewNoop is not usable here. Its Contains reports false, which
// reads as "not blocked" for a blocklist but as "not allowed" for an
// allowlist — mtglib's Serve rejects any address the allowlist does not
// contain, so a noop one would refuse every connection.
type allowAll struct{}

func (allowAll) Contains(net.IP) bool { return true }
func (allowAll) Run(time.Duration)    {}
func (allowAll) Shutdown()            {}

// Config is everything the proxy needs that does not come from the roster.
type Config struct {
	Listen             string // "host:port" the proxy binds
	MaskingDomain      string // the site FakeTLS impersonates
	Concurrency        uint
	IdleTimeout        time.Duration
	PreferIP           string
	DomainFrontingPort uint
}

// Status is what the panel's header strip shows.
type Status struct {
	Running   bool      `json:"running"`
	Listen    string    `json:"listen"`
	Clients   int       `json:"clients"`
	Connected int       `json:"connected"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

// Manager owns the listener, the mtglib proxy and the event stream. One
// instance per RouteBox process.
type Manager struct {
	mu     sync.Mutex
	store  *Store
	cfg    Config
	proxy  *mtglib.Proxy
	lis    net.Listener
	events *EventStream

	startedAt time.Time
	clients   int
}

// NewManager constructs a stopped manager over a roster.
func NewManager(store *Store) *Manager {
	return &Manager{store: store}
}

// Store exposes the roster to the API layer, mirroring awg.Manager.Store.
func (m *Manager) Store() *Store { return m.store }

// Events exposes the stream so the traffic flusher and the connections
// endpoint can read it. Nil until the first successful Start.
func (m *Manager) Events() *EventStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.events
}

// Start builds a proxy from the current roster and begins serving.
func (m *Manager) Start(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.startLocked(cfg)
}

func (m *Manager) startLocked(cfg Config) error {
	if m.proxy != nil {
		return ErrAlreadyRunning
	}

	secrets, names, err := m.secretsLocked(cfg.MaskingDomain)
	if err != nil {
		return err
	}

	events := NewEventStream(names)

	// network/v2 is what mtg's own cli uses; the non-v2 package exports no New.
	// A nil URL asks for mtg's caching resolver.
	resolver, err := network.GetDNS(nil)
	if err != nil {
		return fmt.Errorf("cannot build resolver: %w", err)
	}

	// The zero timeouts mean "use mtg's defaults" rather than "no timeout".
	ntw := network.New(
		resolver,
		"",
		0,
		0,
		cfg.IdleTimeout,
		net.KeepAliveConfig{Enable: true},
		network.DefaultTCPNotSentLowat,
	)

	proxy, err := mtglib.NewProxy(mtglib.ProxyOpts{
		Secrets: secrets,
		Network: ntw,
		AntiReplayCache: antireplay.NewStableBloomFilter(
			antireplay.DefaultStableBloomFilterMaxSize,
			antireplay.DefaultStableBloomFilterErrorRate,
		),
		IPBlocklist:        ipblocklist.NewNoop(),
		IPAllowlist:        allowAll{},
		EventStream:        events,
		Logger:             newLogger(),
		Concurrency:        cfg.Concurrency,
		IdleTimeout:        cfg.IdleTimeout,
		PreferIP:           cfg.PreferIP,
		DomainFrontingPort: cfg.DomainFrontingPort,
	})
	if err != nil {
		return fmt.Errorf("cannot build proxy: %w", err)
	}

	lis, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		// Shut the proxy down rather than leaving its background goroutines
		// running for a listener that never existed.
		proxy.Shutdown()

		return fmt.Errorf("cannot listen on %s: %w", cfg.Listen, err)
	}

	m.proxy = proxy
	m.lis = lis
	m.events = events
	m.cfg = cfg
	m.clients = len(secrets)
	m.startedAt = time.Now()

	go func() {
		// Serve returns when the listener closes, which is what Stop does.
		_ = proxy.Serve(lis)
	}()

	return nil
}

// Stop shuts the proxy down and releases the port.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	if m.proxy == nil {
		return nil
	}

	// The listener closes first: Shutdown explicitly does not close it, and
	// leaving it open would make the next Start fail on a busy port.
	err := m.lis.Close()
	m.proxy.Shutdown()

	m.proxy = nil
	m.lis = nil
	m.clients = 0

	return err
}

// Rebuild reapplies the roster to a running proxy, and is a no-op when stopped.
//
// It is a stop-and-start because mtglib cannot swap a live secret list. That is
// the right trade: the alternative is continuing to serve a secret the operator
// just revoked. Existing connections are dropped, which is exactly what
// revocation means.
//
// The whole thing holds the lock so a concurrent Start cannot slip into the gap
// where the proxy is stopped.
func (m *Manager) Rebuild() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proxy == nil {
		return nil
	}

	cfg := m.cfg

	if err := m.stopLocked(); err != nil {
		return err
	}

	// A failure here leaves the proxy stopped, which is the safe direction:
	// deleting the last client must not leave its secret still being served.
	return m.startLocked(cfg)
}

// Status reports what the panel header shows.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := Status{
		Running: m.proxy != nil,
		Listen:  m.cfg.Listen,
		Clients: m.clients,
	}

	if status.Running {
		status.StartedAt = m.startedAt
	}

	if m.events != nil {
		status.Connected = len(m.events.Connections())
	}

	return status
}

// secretsLocked turns the active roster into an mtglib secret list plus the
// parallel name list the event stream indexes by.
//
// Their shared order is the contract that makes EventClientMatched meaningful:
// index i in one is index i in the other.
func (m *Manager) secretsLocked(maskingDomain string) ([]mtglib.Secret, []string, error) {
	if maskingDomain == "" {
		return nil, nil, ErrNoMaskingDomain
	}

	active := m.store.Active(time.Now())
	if len(active) == 0 {
		return nil, nil, ErrNoActiveClients
	}

	secrets := make([]mtglib.Secret, 0, len(active))
	names := make([]string, 0, len(active))

	for _, c := range active {
		raw, err := hex.DecodeString(c.Secret)
		if err != nil || len(raw) != SecretKeyBytes {
			// Named, because "invalid secret" against a roster of thirty is
			// not something an operator can act on.
			return nil, nil, fmt.Errorf("client %q has a malformed secret", c.Name)
		}

		var secret mtglib.Secret

		copy(secret.Key[:], raw)
		secret.Host = maskingDomain

		secrets = append(secrets, secret)
		names = append(names, c.Name)
	}

	return secrets, names, nil
}

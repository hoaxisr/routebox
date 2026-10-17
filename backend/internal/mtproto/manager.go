package mtproto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/9seconds/mtg/v2/antireplay"
	"github.com/9seconds/mtg/v2/ipblocklist"
	"github.com/9seconds/mtg/v2/mtglib"
	"github.com/9seconds/mtg/v2/mtglib/web"
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

	// SocksProxy routes the proxy's Telegram connections through a SOCKS5
	// proxy — in RouteBox, the managed loopback inbound that sing-box pins to
	// whichever outbound the operator chose. Empty means dial Telegram
	// directly, which is the default and what every install did before this
	// existed.
	SocksProxy string

	// Web enables the Telegram WEB proxy next to Fake TLS. Nil means Fake TLS
	// only.
	Web *WebConfig
}

// WebConfig is the WEB proxy half of Config. Nil means Fake TLS only.
type WebConfig struct {
	// Hostname is the public DNS name clients reach the WEB proxy on. It must
	// be canonical (lowercase, no port, no trailing dot); the handler refuses
	// anything else, and so does Start.
	Hostname string

	// TrustedProxies are peers allowed to set X-Forwarded-For. Loopback is
	// always trusted on top of these, because the web front lives there.
	TrustedProxies []netip.Prefix

	// Public is what strangers get: every request without an authentic
	// secret. Never nil.
	Public http.Handler
}

// WebFront is where the manager publishes its handler; webfront.Front
// satisfies it. An interface keeps this package free of the front's imports.
type WebFront interface{ SetWebHandler(http.Handler) }

// Status is what the panel's header strip shows.
type Status struct {
	Running   bool      `json:"running"`
	Listen    string    `json:"listen"`
	Clients   int       `json:"clients"`
	Connected int       `json:"connected"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Web       WebStatus `json:"web"`
}

// WebStatus is the WEB proxy's share of the header strip.
type WebStatus struct {
	Enabled  bool `json:"enabled"`
	Sessions int  `json:"sessions"`
	Streams  int  `json:"streams"`
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

	// logs outlives any single proxy: a viewer opened before the last restart
	// should still see what led up to it.
	logs *LogBuffer

	// served closes when the accept loop has returned, so Stop can order
	// itself against it.
	served chan struct{}

	startedAt time.Time
	clients   int

	// web is the WEB proxy handler of the running proxy, nil when Fake TLS
	// only or stopped. front is where it is published.
	web   *web.Handler
	front WebFront

	// tokenKey signs the handler's bootstrap and session tokens. It is per
	// manager, not per Start, so a Rebuild does not log every WEB client out.
	tokenKey []byte

	// networkOverride replaces the network buildNetwork would pick. A test
	// seam only, never set in production: it lets the end-to-end test send
	// the proxy's Telegram dials to an in-process fake datacenter.
	networkOverride mtglib.Network
}

// NewManager constructs a stopped manager over a roster.
func NewManager(store *Store) *Manager {
	key := make([]byte, 32)

	// A broken RNG is not recoverable, and a guessable signing key would let
	// anyone mint sessions, so refuse to run rather than limp on.
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("mtproto: cannot read random bytes: %v", err))
	}

	return &Manager{store: store, logs: NewLogBuffer(DefaultLogBufferSize), tokenKey: key}
}

// SetWebFront sets where the WEB proxy handler is published. Call it before
// Start; a handler already published is not moved.
func (m *Manager) SetWebFront(f WebFront) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.front = f
}

// Logs exposes the proxy's recent log lines, so the panel can show what the
// proxy is doing without the operator reaching for `docker logs`.
func (m *Manager) Logs() *LogBuffer { return m.logs }

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

	ntw, err := buildNetwork(cfg)
	if err != nil {
		return err
	}

	if m.networkOverride != nil {
		ntw = m.networkOverride
	}

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
		Logger:             newBufferedLogger(m.logs),
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

	var handler *web.Handler

	if cfg.Web != nil {
		handler, err = buildWebHandler(cfg.Web, secrets, proxy, m.tokenKey, m.logs)
		if err != nil {
			_ = lis.Close()
			proxy.Shutdown()

			return fmt.Errorf("cannot build web proxy: %w", err)
		}
	}

	served := make(chan struct{})

	m.web = handler

	if handler != nil && m.front != nil {
		m.front.SetWebHandler(handler)
	}

	m.proxy = proxy
	m.lis = lis
	m.events = events
	m.cfg = cfg
	m.clients = len(secrets)
	m.startedAt = time.Now()
	m.served = served

	go func() {
		defer close(served)

		// Serve returns when the listener closes, which is what Stop does.
		_ = proxy.Serve(lis)
	}()

	return nil
}

// buildWebHandler builds the WEB proxy handler over the same secret list the
// proxy got, so profile index i is secret i and the event stream's client
// names line up with it.
func buildWebHandler(
	cfg *WebConfig,
	secrets []mtglib.Secret,
	proxy *mtglib.Proxy,
	tokenKey []byte,
	logs *LogBuffer,
) (*web.Handler, error) {
	profiles := make([]web.Profile, 0, len(secrets))

	for i, s := range secrets {
		profiles = append(profiles, web.Profile{Index: i, Secret: s})
	}

	trusted := []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}
	trusted = append(trusted, cfg.TrustedProxies...)

	return web.NewHandler(web.Options{
		Hostname:       cfg.Hostname,
		Profiles:       profiles,
		Serve:          proxy.ServeWebConn,
		Public:         cfg.Public,
		TrustedProxies: trusted,
		TokenKey:       tokenKey,
		Logger:         newBufferedLogger(logs).Named("web"),
	})
}

// buildNetwork picks how the proxy reaches Telegram: straight out of the box, or
// through the managed SOCKS inbound and out whichever outbound the operator
// chose on the Telegram page.
func buildNetwork(cfg Config) (mtglib.Network, error) {
	if cfg.SocksProxy != "" {
		return newSocksNetwork(cfg.SocksProxy, cfg.IdleTimeout), nil
	}

	// network/v2 is what mtg's own cli uses; the non-v2 package exports no New.
	// A nil URL asks for mtg's caching resolver.
	resolver, err := network.GetDNS(nil)
	if err != nil {
		return nil, fmt.Errorf("cannot build resolver: %w", err)
	}

	// The zero timeouts mean "use mtg's defaults" rather than "no timeout".
	return network.New(
		resolver,
		"",
		0,
		0,
		cfg.IdleTimeout,
		net.KeepAliveConfig{Enable: true},
		network.DefaultTCPNotSentLowat,
	), nil
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

	// Then wait for the accept loop to actually be gone before shutting down.
	//
	// Serve registers itself with the proxy's internal wait group on entry, and
	// Shutdown waits on that group. Stopping a proxy whose serve goroutine has
	// not been scheduled yet would have Shutdown wait on a counter that is
	// about to be incremented — a real race, and one the detector flags. With
	// the listener already closed, Accept fails immediately, so this returns
	// promptly whether or not Serve had started.
	<-m.served

	// The web handler goes before the proxy: its sessions feed streams into
	// the proxy, and the front must stop sending it requests first. Reverting
	// the front hands strangers the stub site while the handler drains, and
	// the drain is bounded so a stuck stream cannot hold Stop forever.
	m.detachWebLocked()

	m.proxy.Shutdown()

	m.proxy = nil
	m.lis = nil
	m.served = nil
	m.clients = 0

	return err
}

// detachWebLocked reverts the front to the stub and drains the WEB handler,
// if there is one. Closing its sessions releases every parked long poll, so
// the requests the front is holding for it end at once rather than when their
// poll runs out.
func (m *Manager) detachWebLocked() {
	if m.web == nil {
		return
	}

	if m.front != nil {
		m.front.SetWebHandler(nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), web.DefaultTimeouts().Shutdown)
	// Logged, not returned: the proxy still has to stop, and a drain that ran
	// out of time only means some WEB sessions were cut short.
	if err := m.web.Shutdown(ctx); err != nil {
		log.Printf("mtproto: WEB proxy shutdown: %v", err)
	}

	cancel()

	m.web = nil
}

// DetachWeb stops serving the WEB proxy on a running proxy and leaves Fake TLS
// up; a no-op when there is no WEB handler. It exists for the front's sake:
// stopping a front that still hands requests to the handler waits for every
// parked long poll in it, up to the front's whole drain timeout, while a
// detached handler has already released them.
//
// The saved config forgets WEB too, so a Rebuild in the meantime does not
// bring it back; the next Start or Reconfigure decides whether it returns.
func (m *Manager) DetachWeb() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.detachWebLocked()
	m.cfg.Web = nil
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

// Reconfigure applies a new Config to a running proxy, and is a no-op when
// stopped.
//
// Rebuild reuses the Config saved at Start, so a settings change (listen
// address, masking domain, outbound, the WEB keys) would never reach a running
// proxy through it. Like Rebuild, a failure leaves the proxy stopped.
func (m *Manager) Reconfigure(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proxy == nil {
		return nil
	}

	if err := m.stopLocked(); err != nil {
		return err
	}

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

	if m.web != nil {
		stats := m.web.Stats()

		status.Web = WebStatus{Enabled: true, Sessions: stats.SessionsLive, Streams: stats.StreamsLive}
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

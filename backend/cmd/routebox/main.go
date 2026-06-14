package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"routebox/backend/internal/api"
	"routebox/backend/internal/auth"
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/embedded"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/updates"
	"routebox/backend/internal/users"
)

// Version is stamped at build time via -ldflags "-X main.Version=…".
// "dev" for plain `go build` / `go run`.
var Version = "dev"

var (
	settingsPath = flag.String("settings", "", "Path to routebox.toml settings file (auto-detected if not specified)")
	// Legacy flags - override settings file if specified
	configPath = flag.String("config", "", "Path to amnezia-box config file (overrides settings)")
	listenAddr = flag.String("listen", "", "HTTP listen address (overrides settings)")
	clashAddr  = flag.String("clash", "", "Clash API address (overrides settings)")
	geoipPath  = flag.String("geoip", "", "Path to GeoIP MMDB database (overrides settings)")
	modeFlag   = flag.String("mode", "", "panel mode: router (default) or vps")
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("routebox version %s\n", Version)
		return
	}
	flag.Parse()

	// Require root privileges (needed for TUN interface)
	if os.Geteuid() != 0 {
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
		fmt.Println("║  ERROR: Root privileges required                                 ║")
		fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
		fmt.Println("║  routebox must run as root to create TUN interfaces.             ║")
		fmt.Println("║                                                                  ║")
		fmt.Println("║  Please run with sudo:                                           ║")
		fmt.Println("║     sudo ./routebox                                              ║")
		fmt.Println("║                                                                  ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
		fmt.Println()
		os.Exit(1)
	}

	// Load settings
	settingsMgr, err := settings.NewManager(*settingsPath)
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}
	cfg := settingsMgr.Get()

	// Resolve effective mode: CLI flag > settings > default
	effectiveMode := cfg.Server.Mode
	if *modeFlag != "" {
		effectiveMode = *modeFlag
	}
	if effectiveMode == "" {
		effectiveMode = "router"
	}

	// Print settings info
	if settingsMgr.GetPath() != "" {
		fmt.Printf("Settings: %s\n", settingsMgr.GetPath())
	}

	// Initialize process manager
	procMgr := process.NewManager()

	// Resolve config path: CLI flag > settings > auto-detect > default
	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		resolvedConfigPath = cfg.Singbox.ConfigPath
	}
	if resolvedConfigPath == "" {
		// Try to detect from running process or systemd service
		if detected := procMgr.GetDetectedConfigPath(); detected != "" {
			resolvedConfigPath = detected
			fmt.Printf("Auto-detected config path: %s\n", resolvedConfigPath)
		} else {
			// Fallback to default (try Entware path first)
			for _, fallback := range []string{
				"/opt/etc/sing-box/config.json",
				"/opt/etc/amnezia-box/config.json",
				"/etc/amnezia-box/config.json",
			} {
				if _, err := os.Stat(filepath.Dir(fallback)); err == nil {
					resolvedConfigPath = fallback
					break
				}
			}
			if resolvedConfigPath == "" {
				resolvedConfigPath = "/etc/amnezia-box/config.json"
			}
		}
	}

	// Ensure config directory exists
	configDir := filepath.Dir(resolvedConfigPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Failed to create config directory %s: %v", configDir, err)
	}

	// Initialize config manager
	var cfgMgr *config.Manager

	if _, statErr := os.Stat(resolvedConfigPath); statErr == nil {
		// Config exists - load it
		var err error
		cfgMgr, err = config.NewManager(resolvedConfigPath)
		if err != nil {
			log.Printf("Warning: Could not load config from %s: %v", resolvedConfigPath, err)
			cfgMgr = config.NewEmptyManager(resolvedConfigPath)
		}
	} else {
		// Config doesn't exist - create empty manager (setup wizard will create it)
		cfgMgr = config.NewEmptyManager(resolvedConfigPath)
	}

	// Clean up any stale draft from previous session
	cfgMgr.CleanupDraft()

	// Use the currently detected sing-box/amnezia-box binary for config
	// validation — provider, not a snapshot, so later detection is picked up
	cfgMgr.SetCheckBinaryProvider(procMgr.GetBinaryPath)

	// Set config path for process manager
	procMgr.SetConfigPath(resolvedConfigPath)

	// Resolve Clash API address: CLI flag > settings > auto-detect
	resolvedClashAddr := *clashAddr
	if resolvedClashAddr == "" {
		resolvedClashAddr = cfg.Singbox.ClashAPI
	}
	if resolvedClashAddr == "" {
		resolvedClashAddr = resolveClashAddrFromConfig(cfgMgr)
	}

	// Resolve GeoIP path: CLI flag > settings
	resolvedGeoipPath := *geoipPath
	if resolvedGeoipPath == "" {
		resolvedGeoipPath = cfg.GeoIP.Path
	}

	// Load GeoIP database if specified
	var geoipDB *geoip.DB
	if resolvedGeoipPath != "" {
		var err error
		geoipDB, err = geoip.Open(resolvedGeoipPath)
		if err != nil {
			log.Printf("Warning: Could not load GeoIP database from %s: %v", resolvedGeoipPath, err)
		} else {
			fmt.Printf("GeoIP database: %s\n", resolvedGeoipPath)
		}
	}

	// Resolve listen address: CLI flag > settings
	resolvedListenAddr := *listenAddr
	if resolvedListenAddr == "" {
		resolvedListenAddr = cfg.Network.Listen
	}

	// Initialize clients manager (LAN device names)
	var clientsPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		clientsPath = filepath.Join(filepath.Dir(sp), "clients.toml")
	}
	clientsMgr := clients.New(clientsPath)
	if clientsPath != "" {
		if err := clientsMgr.Load(); err != nil {
			log.Printf("Warning: failed to load clients.toml: %v", err)
		}
	}
	stopClients := make(chan struct{})
	clientsDone := make(chan struct{})
	go clientsMgr.StartPersistLoop(30*time.Second, stopClients, clientsDone)

	// Auto-discover LAN clients from Clash /connections
	stopDiscovery := make(chan struct{})
	go runClientDiscovery(clientsMgr, resolvedClashAddr, stopDiscovery)

	// Open traffic history store (next to settings file)
	var trafficStore *traffic.Store
	if sp := settingsMgr.GetPath(); sp != "" {
		trafficPath := filepath.Join(filepath.Dir(sp), "traffic.db")
		if ts, err := traffic.OpenStore(trafficPath); err != nil {
			log.Printf("Warning: traffic store unavailable: %v", err)
		} else {
			trafficStore = ts
		}
	}

	// Start traffic sampler (no-op if trafficStore is nil)
	stopSampler := make(chan struct{})
	if trafficStore != nil {
		sampler := traffic.NewSampler(trafficStore)
		go sampler.Run(resolvedClashAddr, 35, stopSampler)
	}

	// Updates: GitHub release checker + daily auto-check (Task 4 adds API)
	updChecker := updates.NewChecker()
	updUpdater := updates.NewUpdater()
	updTargets := []updates.Target{
		updates.AmneziaTarget(procMgr.GetBinaryPath, procMgr.GetVersion, func() error {
			return procMgr.Restart(cfgMgr.GetPath())
		}),
		updates.RouteBoxTarget(Version),
	}
	stopUpdates := make(chan struct{})
	go updates.RunDailyChecks(updChecker, updTargets, func() bool {
		return settingsMgr.Get().Updates.AutoCheck
	}, 24*time.Hour, stopUpdates)

	// Subscriptions: TOML store + hourly auto-refresh of due subscriptions.
	var subsPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		subsPath = filepath.Join(filepath.Dir(sp), "subscriptions.toml")
	}
	subsMgr := subscriptions.NewManager(subsPath)
	if subsPath != "" {
		if err := subsMgr.Load(); err != nil {
			log.Printf("Warning: failed to load subscriptions.toml: %v", err)
		}
	}
	subsRefresh := func(s subscriptions.Subscription) (int, int, error) {
		return subscriptions.Refresh(s, cfgMgr)
	}
	stopSubs := make(chan struct{})
	go subscriptions.RunRefreshLoop(subsMgr, cfgMgr, subscriptions.Refresh, time.Hour, stopSubs)

	// Panel users: sidecar registry mirroring the active config.
	var usersPath string
	if sp := settingsMgr.GetPath(); sp != "" {
		usersPath = filepath.Join(filepath.Dir(sp), "users.toml")
	}
	usersMgr := users.NewManager(usersPath)
	if usersPath != "" {
		if err := usersMgr.Load(); err != nil {
			log.Printf("Warning: failed to load users.toml: %v", err)
		}
	}
	// Startup reconcile against the active config (additive: empty active -> no-op).
	if _, err := usersMgr.Reconcile(cfgMgr.GetActive()); err != nil {
		log.Printf("Warning: startup users reconcile failed: %v", err)
	}

	// Initialize API handlers
	apiHandler := api.NewHandler(cfgMgr, procMgr, resolvedClashAddr, geoipDB, settingsMgr, clientsMgr, trafficStore)
	apiHandler.SetRouteBoxVersion(Version)
	apiHandler.SetUpdatesService(&updates.Service{
		Checker: updChecker,
		Updater: updUpdater,
		Targets: updTargets,
	})
	apiHandler.SetSubscriptions(subsMgr, subsRefresh)
	apiHandler.SetUsers(usersMgr)

	// Dedicated per-IP rate-limiter for the PUBLIC /sub/{token} endpoint, kept
	// separate from the auth/lockout limiter. Keyed on clientIP(r) = RemoteAddr.
	subLimiter := auth.NewLimiter()
	apiHandler.SetSubLimiter(subLimiter)

	// Construct auth deps, wire onto handler, start cleanup ticker
	sessions := auth.NewSessionStore()
	limiter := auth.NewLimiter()
	verifier := auth.NewCachedVerifier()
	apiHandler.SetAuth(sessions, limiter, verifier)
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			sessions.Cleanup()
			limiter.Cleanup()
			subLimiter.Cleanup()
		}
	}()

	// Setup router
	r := chi.NewRouter()

	// Middleware
	// Root access log with /sub/<token> scrubbing — the subscription token is a
	// credential and must NEVER reach the log (chi's middleware.Logger formats its
	// line from r.RequestURI, which would leak it). All other paths log verbatim.
	r.Use(api.SubTokenScrubber(log.Default()))
	r.Use(middleware.Recoverer)
	if cfg.Network.CompressionEnabled {
		r.Use(middleware.Compress(5))
	}
	if cfg.Security.CorsOrigins != "" {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{cfg.Security.CorsOrigins},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Content-Type"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	// API routes: public group + protected group
	r.Route("/api", func(r chi.Router) {
		// PUBLIC: reachable without auth
		r.Group(func(r chi.Router) {
			r.Post("/auth/login", apiHandler.Login)
			r.Post("/auth/logout", apiHandler.Logout)
			r.Get("/auth/session", apiHandler.Session)
			r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
			})
		})
		// PROTECTED: everything else
		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddleware(settingsMgr, sessions, limiter, verifier))

			// Config (full)
			r.Get("/config", apiHandler.GetConfig)
			r.Put("/config", apiHandler.SaveConfig)
			r.Post("/config/validate", apiHandler.ValidateConfig)
			r.Post("/config/diff", apiHandler.GetConfigDiff)
			r.Post("/config/apply", apiHandler.ApplyConfig)
			r.Get("/config/export", apiHandler.ExportConfig)
			r.Post("/config/import", apiHandler.ImportConfig)

			// Config draft system
			r.Get("/config/status", apiHandler.GetConfigStatus)
			r.Post("/config/discard", apiHandler.DiscardConfig)
			r.Get("/config/draft-diff", apiHandler.GetDraftDiff)
			r.Post("/config/save", apiHandler.SaveConfigDraft)
			r.Post("/config/check", apiHandler.CheckConfig)
			r.Get("/config/active", apiHandler.GetActiveConfig)

			// Endpoints CRUD
			r.Route("/endpoints", func(r chi.Router) {
				r.Get("/", apiHandler.ListEndpoints)
				r.Post("/", apiHandler.CreateEndpoint)
				r.Get("/{tag}", apiHandler.GetEndpoint)
				r.Put("/{tag}", apiHandler.UpdateEndpoint)
				r.Delete("/{tag}", apiHandler.DeleteEndpoint)
			})

			// Outbounds CRUD
			r.Route("/outbounds", func(r chi.Router) {
				r.Get("/", apiHandler.ListOutbounds)
				r.Post("/", apiHandler.CreateOutbound)
				r.Get("/{tag}", apiHandler.GetOutbound)
				r.Put("/{tag}", apiHandler.UpdateOutbound)
				r.Delete("/{tag}", apiHandler.DeleteOutbound)
			})

			// Inbounds CRUD
			r.Route("/inbounds", func(r chi.Router) {
				r.Get("/", apiHandler.ListInbounds)
				r.Post("/", apiHandler.CreateInbound)
				r.Get("/{tag}", apiHandler.GetInbound)
				r.Put("/{tag}", apiHandler.UpdateInbound)
				r.Delete("/{tag}", apiHandler.DeleteInbound)
			})

			// Credential generators (server inbounds)
			r.Route("/generate", func(r chi.Router) {
				r.Post("/reality", apiHandler.GenerateReality)
				r.Post("/uuid", apiHandler.GenerateUUID)
				r.Post("/password", apiHandler.GeneratePassword)
			})

			// Route Rule Sets CRUD
			r.Route("/route/rule-sets", func(r chi.Router) {
				r.Get("/", apiHandler.ListRuleSets)
				r.Post("/", apiHandler.CreateRuleSet)
				r.Get("/usage", apiHandler.GetRuleSetsUsage)
				r.Put("/{tag}", apiHandler.UpdateRuleSet)
				r.Delete("/{tag}", apiHandler.DeleteRuleSet)
			})

			// Domain Sets (custom rule set sources)
			r.Route("/domains", func(r chi.Router) {
				r.Get("/", apiHandler.ListDomainSets)
				r.Post("/", apiHandler.CreateDomainSet)
				r.Route("/{tag}", func(r chi.Router) {
					r.Get("/", apiHandler.GetDomainSet)
					r.Put("/", apiHandler.SaveDomainSet)
					r.Delete("/", apiHandler.DeleteDomainSet)
					r.Post("/domain", apiHandler.AddDomain)
					r.Delete("/domain/{domain}", apiHandler.RemoveDomain)
					r.Post("/import", apiHandler.ImportDomains)
				})
			})

			// Clients (LAN device names)
			r.Route("/clients", func(r chi.Router) {
				r.Get("/", apiHandler.ListClients)
				r.Put("/{ip}", apiHandler.UpdateClient)
				r.Delete("/{ip}", apiHandler.DeleteClient)
			})

			// Subscriptions CRUD + refresh
			r.Route("/subscriptions", func(r chi.Router) {
				r.Get("/", apiHandler.ListSubscriptions)
				r.Post("/", apiHandler.CreateSubscription)
				r.Put("/{id}", apiHandler.UpdateSubscription)
				r.Delete("/{id}", apiHandler.DeleteSubscription)
				r.Post("/{id}/refresh", apiHandler.RefreshSubscription)
			})

			// Panel users CRUD + share-by-id
			r.Route("/users", func(r chi.Router) {
				r.Get("/", apiHandler.ListUsers)
				r.Post("/", apiHandler.CreateUser)
				r.Delete("/{id}", apiHandler.DeleteUser)
				r.Post("/{id}/bindings", apiHandler.AddBinding)
				r.Get("/{id}/link", apiHandler.GetUserLinkByID)
				r.Post("/{id}/token/rotate", apiHandler.RotateUserToken)
				r.Delete("/{id}/token", apiHandler.RevokeUserToken)
			})

			// Route Rules CRUD
			r.Route("/route/rules", func(r chi.Router) {
				r.Get("/", apiHandler.ListRules)
				r.Post("/", apiHandler.CreateRule)
				r.Put("/{index}", apiHandler.UpdateRule)
				r.Delete("/{index}", apiHandler.DeleteRule)
				r.Put("/reorder", apiHandler.ReorderRules)
			})

			// Route Settings
			r.Get("/route/settings", apiHandler.GetRouteSettings)
			r.Put("/route/settings", apiHandler.UpdateRouteSettings)

			// Route Inspector
			r.Post("/route/test", apiHandler.TestRoute)

			// Connection Test (diagnostics)
			r.Post("/diagnostics/connect", apiHandler.ConnectTest)

			// Traffic history (SQLite-backed)
			r.Get("/traffic/history", apiHandler.GetTrafficHistory)
			r.Post("/traffic/reset", apiHandler.ResetTrafficHistory)

			// DNS Servers CRUD
			r.Route("/dns/servers", func(r chi.Router) {
				r.Get("/", apiHandler.ListDnsServers)
				r.Post("/", apiHandler.CreateDnsServer)
				r.Put("/{tag}", apiHandler.UpdateDnsServer)
				r.Delete("/{tag}", apiHandler.DeleteDnsServer)
			})

			// DNS Rules CRUD
			r.Route("/dns/rules", func(r chi.Router) {
				r.Get("/", apiHandler.ListDnsRules)
				r.Post("/", apiHandler.CreateDnsRule)
				r.Put("/{index}", apiHandler.UpdateDnsRule)
				r.Delete("/{index}", apiHandler.DeleteDnsRule)
				r.Put("/reorder", apiHandler.ReorderDnsRules)
			})

			// DNS Settings
			r.Get("/dns/settings", apiHandler.GetDnsSettings)
			r.Put("/dns/settings", apiHandler.UpdateDnsSettings)

			// Log Settings
			r.Get("/log", apiHandler.GetLogSettings)
			r.Put("/log", apiHandler.UpdateLogSettings)

			// Experimental Settings
			r.Get("/experimental", apiHandler.GetExperimental)
			r.Put("/experimental", apiHandler.UpdateExperimental)

			// Version & Feature Flags
			r.Get("/version", apiHandler.GetVersion)

			// Binary updates (amnezia-box + RouteBox self-update)
			r.Route("/updates", func(r chi.Router) {
				r.Get("/status", apiHandler.GetUpdatesStatus)
				r.Post("/check", apiHandler.CheckUpdates)
				r.Post("/apply", apiHandler.ApplyUpdate)
				r.Get("/progress", apiHandler.GetUpdatesProgress)
			})

			// Status & Control
			r.Get("/status", apiHandler.GetStatus)
			r.Post("/control/start", apiHandler.Start)
			r.Post("/control/stop", apiHandler.Stop)
			r.Post("/control/restart", apiHandler.Restart)
			r.Post("/control/reload", apiHandler.Reload)

			// Config detection (kept for compatibility)
			r.Get("/config/detected", apiHandler.GetDetectedConfig)
			r.Post("/config/use-detected", apiHandler.UseDetectedConfig)

			// Systemd logs
			r.Get("/logs/journal", apiHandler.GetJournalLogs)

			// Clash API proxy - WebSocket routes MUST be registered before the wildcard
			r.HandleFunc("/clash/traffic", apiHandler.ProxyClashWebSocket)
			r.HandleFunc("/clash/logs", apiHandler.ProxyClashWebSocket)
			r.HandleFunc("/clash/connections", apiHandler.ProxyClashWebSocket)
			r.Get("/clash/*", apiHandler.ProxyClashAPI)
			r.Delete("/clash/*", apiHandler.ProxyClashAPI)

			// RouteBox Settings API
			r.Get("/settings", apiHandler.GetSettings)
			r.Put("/settings", apiHandler.UpdateSettings)
			r.Post("/settings/reload", apiHandler.ReloadSettings)

			// Setup wizard
			r.Get("/needs-setup", apiHandler.NeedsSetup)
		})
	})

	// PUBLIC per-user subscription — registered OUTSIDE the /api auth group so
	// clients (which have no panel credentials) can fetch it. Anti-enumeration,
	// per-IP rate-limit, the 503-no-host policy, and log scrubbing are enforced in
	// the handler / root middleware. Sibling of /api and the SPA wildcard; chi's
	// longest-prefix match makes this win over /*.
	r.Get("/sub/{token}", apiHandler.GetSubscription)

	// Serve embedded frontend (SPA)
	r.Get("/*", embedded.Handler())

	// VPS-mode bootstrap (force-auth) + router-mode warning
	if effectiveMode == "vps" {
		sec := settingsMgr.Get().Security
		if !sec.AuthEnabled || sec.AuthPasswordHash == "" {
			pw, err := generatePassword()
			if err != nil {
				log.Fatalf("generate bootstrap password: %v", err)
			}
			if err := settingsMgr.Update(map[string]interface{}{
				"security.auth_enabled":  true,
				"security.auth_username": orDefault(sec.AuthUsername, "admin"),
				"security.auth_password": pw,
			}); err != nil {
				log.Fatalf("enable auth: %v", err)
			}
			if err := settingsMgr.Save(); err != nil {
				log.Fatalf("persist auth: %v", err)
			}
			pwFile := filepath.Join(filepath.Dir(settingsMgr.GetPath()), "routebox-initial-password")
			writeErr := os.WriteFile(pwFile, []byte(pw+"\n"), 0600)
			fmt.Println("==================================================================")
			fmt.Println(" VPS MODE: panel auth was OFF — generated admin credentials")
			fmt.Printf("   username: %s\n", orDefault(sec.AuthUsername, "admin"))
			fmt.Printf("   password: %s\n", pw)
			if writeErr == nil {
				fmt.Printf("   (also written to %s)\n", pwFile)
			} else {
				fmt.Printf("   (WARNING: could not write password file: %v)\n", writeErr)
			}
			fmt.Println("==================================================================")
		}
	} else if isNonLoopback(resolvedListenAddr) && !settingsMgr.Get().Security.AuthEnabled {
		log.Printf("WARNING: RouteBox is listening on a non-loopback address (%s) with auth DISABLED — the panel is open to the network. Enable security.auth_enabled or run with --mode=vps.", resolvedListenAddr)
	}

	// Compute TLS usage
	certPath := settingsMgr.Get().Network.TLSCertPath
	keyPath := settingsMgr.Get().Network.TLSKeyPath
	useTLS := certPath != "" && keyPath != ""
	scheme := "http"
	if useTLS {
		scheme = "https"
	}

	// Start server
	fmt.Println()
	fmt.Printf("RouteBox %s starting on %s://%s\n", Version, scheme, resolvedListenAddr)
	fmt.Printf("Config: %s\n", resolvedConfigPath)
	if resolvedClashAddr != "" {
		fmt.Printf("Clash API: %s\n", resolvedClashAddr)
	}
	if !procMgr.IsBinaryInstalled() {
		fmt.Println("Note: amnezia-box not installed - run setup wizard to configure")
	}
	fmt.Println()

	// WriteTimeout is intentionally left at 0 (unlimited) because the Clash
	// WebSocket proxy endpoints (/api/clash/traffic, /clash/logs, /clash/connections)
	// are long-lived streaming connections. A non-zero http.Server.WriteTimeout
	// applies to all handlers globally and would terminate those streams after the
	// deadline, breaking real-time monitoring. ReadTimeout is safe to set because
	// it only covers the time to read the initial request headers/body.
	srv := &http.Server{
		Addr:              resolvedListenAddr,
		Handler:           r,
		ReadTimeout:       time.Duration(cfg.Network.ReadTimeoutSec) * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if useTLS {
			errCh <- srv.ListenAndServeTLS(certPath, keyPath)
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		log.Fatalf("Server error: %v", err)
	case <-ctx.Done():
	}
	stop() // restore default signal behavior: second Ctrl-C kills immediately

	fmt.Println("\nShutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}

	// Stop background loops, then close the store they write to
	close(stopClients)
	<-clientsDone // final clients.toml flush completes before exit
	close(stopDiscovery)
	close(stopSampler)
	close(stopUpdates)
	close(stopSubs)
	if trafficStore != nil {
		trafficStore.Close()
	}
}

// generatePassword returns a 24-character URL-safe random password.
func generatePassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// orDefault returns s if non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// isNonLoopback reports whether addr resolves to a non-loopback interface.
func isNonLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return host != "" && host != "127.0.0.1" && host != "::1" && host != "localhost"
}

// runClientDiscovery polls the Clash /connections endpoint every 60s and feeds
// observed source IPs into the clients manager. Exits cleanly when stop closes.
func runClientDiscovery(mgr *clients.Manager, clashAddr string, stop <-chan struct{}) {
	if clashAddr == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	tick := func() {
		resp, err := client.Get("http://" + clashAddr + "/connections")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var data struct {
			Connections []struct {
				Metadata struct {
					SourceIP string `json:"sourceIP"`
				} `json:"metadata"`
			} `json:"connections"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return
		}
		now := time.Now()
		for _, c := range data.Connections {
			if c.Metadata.SourceIP != "" {
				mgr.Observe(c.Metadata.SourceIP, now)
			}
		}
	}
	tick()
	for {
		select {
		case <-ticker.C:
			tick()
		case <-stop:
			return
		}
	}
}

// resolveClashAddrFromConfig extracts Clash API address from sing-box config
func resolveClashAddrFromConfig(cfgMgr *config.Manager) string {
	cfg := cfgMgr.Get()
	if cfg == nil {
		return ""
	}

	// Look for experimental.clash_api.external_controller
	if exp, ok := cfg["experimental"].(map[string]interface{}); ok {
		if clashApi, ok := exp["clash_api"].(map[string]interface{}); ok {
			if controller, ok := clashApi["external_controller"].(string); ok && controller != "" {
				return controller
			}
		}
	}

	return ""
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"routebox/backend/internal/api"
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/embedded"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/traffic"
)

var (
	settingsPath = flag.String("settings", "", "Path to routebox.toml settings file (auto-detected if not specified)")
	// Legacy flags - override settings file if specified
	configPath = flag.String("config", "", "Path to amnezia-box config file (overrides settings)")
	listenAddr = flag.String("listen", "", "HTTP listen address (overrides settings)")
	clashAddr  = flag.String("clash", "", "Clash API address (overrides settings)")
	geoipPath  = flag.String("geoip", "", "Path to GeoIP MMDB database (overrides settings)")
)

func main() {
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

	// Use the detected sing-box/amnezia-box binary for config validation
	cfgMgr.SetCheckBinary(procMgr.GetBinaryPath())

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
	go clientsMgr.StartPersistLoop(30*time.Second, stopClients)
	defer close(stopClients)

	// Auto-discover LAN clients from Clash /connections
	stopDiscovery := make(chan struct{})
	go runClientDiscovery(clientsMgr, resolvedClashAddr, stopDiscovery)
	defer close(stopDiscovery)

	// Open traffic history store (next to settings file)
	var trafficStore *traffic.Store
	if sp := settingsMgr.GetPath(); sp != "" {
		trafficPath := filepath.Join(filepath.Dir(sp), "traffic.db")
		if ts, err := traffic.OpenStore(trafficPath); err != nil {
			log.Printf("Warning: traffic store unavailable: %v", err)
		} else {
			trafficStore = ts
			defer trafficStore.Close()
		}
	}

	// Start traffic sampler (no-op if trafficStore is nil)
	stopSampler := make(chan struct{})
	if trafficStore != nil {
		sampler := traffic.NewSampler(trafficStore)
		go sampler.Run(resolvedClashAddr, 35, stopSampler)
	}
	defer close(stopSampler)

	// Initialize API handlers
	apiHandler := api.NewHandler(cfgMgr, procMgr, resolvedClashAddr, geoipDB, settingsMgr, clientsMgr, trafficStore)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	if cfg.Network.CompressionEnabled {
		r.Use(middleware.Compress(5))
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.Security.CorsOrigins},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(api.BasicAuth(settingsMgr))

	// API routes
	r.Route("/api", func(r chi.Router) {
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

		// Health
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
	})

	// Serve embedded frontend (SPA)
	r.Get("/*", embedded.Handler())

	// Start server
	fmt.Println()
	fmt.Printf("RouteBox starting on http://%s\n", resolvedListenAddr)
	fmt.Printf("Config: %s\n", resolvedConfigPath)
	if resolvedClashAddr != "" {
		fmt.Printf("Clash API: %s\n", resolvedClashAddr)
	}
	if !procMgr.IsBinaryInstalled() {
		fmt.Println("Note: amnezia-box not installed - run setup wizard to configure")
	}
	fmt.Println()

	srv := &http.Server{
		Addr:              resolvedListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// runClientDiscovery polls the Clash /connections endpoint every 60s and feeds
// observed source IPs into the clients manager. Exits cleanly when stop closes.
func runClientDiscovery(mgr *clients.Manager, clashAddr string, stop <-chan struct{}) {
	if clashAddr == "" {
		return
	}
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	tick := func() {
		resp, err := http.Get("http://" + clashAddr + "/connections")
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

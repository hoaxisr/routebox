package api

import (
	"routebox/backend/internal/auth"
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/updates"
	"routebox/backend/internal/users"
)

// Handler holds API dependencies
type Handler struct {
	config          *config.Manager
	process         *process.Manager
	clashAddr       string
	geoip           *geoip.DB
	settings        *settings.Manager
	clients         *clients.Manager
	traffic         *traffic.Store
	routeboxVersion string
	updates         *updates.Service
	subs            *subscriptions.Manager
	subsRefresh     func(subscriptions.Subscription) (int, int, error)
	sessions        *auth.SessionStore
	limiter         *auth.Limiter
	verifier        *auth.CachedVerifier
	panelUsers      *users.Manager
}

// SetRouteBoxVersion stores the build-time RouteBox version for API responses.
func (h *Handler) SetRouteBoxVersion(v string) {
	h.routeboxVersion = v
}

// SetUpdatesService wires the binary-updates service into the API.
func (h *Handler) SetUpdatesService(s *updates.Service) {
	h.updates = s
}

// SetSubscriptions wires the subscription store and refresh closure into the API.
func (h *Handler) SetSubscriptions(mgr *subscriptions.Manager, refresh func(subscriptions.Subscription) (int, int, error)) {
	h.subs = mgr
	h.subsRefresh = refresh
}

// SetUsers wires the panel-user registry into the API.
func (h *Handler) SetUsers(mgr *users.Manager) {
	h.panelUsers = mgr
}

// SetAuth wires the session store, lockout limiter, and password verifier.
func (h *Handler) SetAuth(s *auth.SessionStore, l *auth.Limiter, v *auth.CachedVerifier) {
	h.sessions = s
	h.limiter = l
	h.verifier = v
}

// NewHandler creates a new API handler
func NewHandler(cfg *config.Manager, proc *process.Manager, clashAddr string, geoipDB *geoip.DB, settingsMgr *settings.Manager, clientsMgr *clients.Manager, trafficStore *traffic.Store) *Handler {
	return &Handler{
		config:    cfg,
		process:   proc,
		clashAddr: clashAddr,
		geoip:     geoipDB,
		settings:  settingsMgr,
		clients:   clientsMgr,
		traffic:   trafficStore,
	}
}

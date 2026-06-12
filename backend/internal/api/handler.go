package api

import (
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/traffic"
	"routebox/backend/internal/updates"
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
}

// SetRouteBoxVersion stores the build-time RouteBox version for API responses.
func (h *Handler) SetRouteBoxVersion(v string) {
	h.routeboxVersion = v
}

// SetUpdatesService wires the binary-updates service into the API.
func (h *Handler) SetUpdatesService(s *updates.Service) {
	h.updates = s
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

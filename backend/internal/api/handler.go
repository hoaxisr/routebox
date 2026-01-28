package api

import (
	"routebox/backend/internal/config"
	"routebox/backend/internal/domains"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
)

// Handler holds API dependencies
type Handler struct {
	config    *config.Manager
	process   *process.Manager
	clashAddr string
	geoip     *geoip.DB
	settings  *settings.Manager
	domains   *domains.Manager
}

// NewHandler creates a new API handler
func NewHandler(cfg *config.Manager, proc *process.Manager, clashAddr string, geoipDB *geoip.DB, settingsMgr *settings.Manager, domainsMgr *domains.Manager) *Handler {
	return &Handler{
		config:    cfg,
		process:   proc,
		clashAddr: clashAddr,
		geoip:     geoipDB,
		settings:  settingsMgr,
		domains:   domainsMgr,
	}
}

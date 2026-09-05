package api

import (
	"routebox/backend/internal/auth"
	"routebox/backend/internal/awg"
	"routebox/backend/internal/clients"
	"routebox/backend/internal/config"
	"routebox/backend/internal/geoip"
	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/sysinfo"
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
	subLimiter      *auth.Limiter
	awg             *awg.Manager
	mtproto         *mtproto.Manager
	sys             sysinfo.Sampler // host metrics for the dashboard; zero value reads /proc

	// panelMode is the effective operating mode ("router" or "vps"), as resolved
	// at startup — the CLI flag can override what the settings file says, so it
	// is passed in rather than read back from settings. Empty means router, the
	// historical default.
	panelMode string

	// dockerMode is true when RouteBox is running inside the official Docker
	// image (ROUTEBOX_RUNTIME=docker). It blocks the RouteBox self-update
	// target from replacing its own binary — the image is the source of
	// truth there — while leaving the amnezia-box target untouched.
	dockerMode bool

	// v2rayAPISupported reports whether the running binary supports the
	// experimental.v2ray_api block (with_v2ray_api build tag). Defaults to
	// h.process.SupportsV2RayAPI when nil; overridable in tests.
	v2rayAPISupported func() bool

	// statusSource yields the process status. Defaults to h.process.GetStatus
	// when nil; overridable in tests so GetStatus can be exercised without a
	// real running process.
	statusSource func() process.Status
}

// getProcessStatus returns the current process status, using the test override
// if set, else the process manager.
func (h *Handler) getProcessStatus() process.Status {
	if h.statusSource != nil {
		return h.statusSource()
	}
	return h.process.GetStatus()
}

// supportsV2RayAPI reports whether the active binary supports the v2ray_api
// block, using the test override if set, else the process manager.
func (h *Handler) supportsV2RayAPI() bool {
	if h.v2rayAPISupported != nil {
		return h.v2rayAPISupported()
	}
	return h.process.SupportsV2RayAPI()
}

// SetRouteBoxVersion stores the build-time RouteBox version for API responses.
func (h *Handler) SetRouteBoxVersion(v string) {
	h.routeboxVersion = v
}

// SetUpdatesService wires the binary-updates service into the API.
func (h *Handler) SetUpdatesService(s *updates.Service) {
	h.updates = s
}

// SetPanelMode records the effective operating mode so the API can leave out
// what does not apply to it — the system requirements below being the case
// that matters: they are about routing a LAN through a TUN interface.
func (h *Handler) SetPanelMode(mode string) {
	h.panelMode = mode
}

// SetDockerMode records whether RouteBox is running inside the official
// Docker image, so the updates API can refuse RouteBox self-replacement and
// point the user at `docker compose pull` instead.
func (h *Handler) SetDockerMode(v bool) {
	h.dockerMode = v
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

// SetAWG wires the AmneziaWG server-interface manager into the API.
func (h *Handler) SetAWG(m *awg.Manager) {
	h.awg = m
}

// SetMtproto wires the Telegram MTProto proxy manager into the API. Leaving it
// nil — which router mode does — makes every /api/mtproto route report 503.
func (h *Handler) SetMtproto(m *mtproto.Manager) {
	h.mtproto = m
}

// SetSubLimiter wires the dedicated per-IP rate limiter for the public /sub
// endpoint. Kept separate from the auth lockout limiter (different keyspace and
// policy: keyed purely by client IP).
func (h *Handler) SetSubLimiter(l *auth.Limiter) {
	h.subLimiter = l
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

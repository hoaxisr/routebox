package api

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/mtproto"
)

// mtprotoClientRow is one roster entry on the wire.
//
// The raw secret is deliberately absent. The page polls this endpoint, and a
// credential that rides along with every poll ends up in every access log and
// proxy buffer; the link endpoint serves it once, on demand.
type mtprotoClientRow struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	Online    bool   `json:"online"`
}

// mtprotoReady guards every handler. Router mode never constructs a manager, so
// the routes have to answer rather than dereference nil.
func (h *Handler) mtprotoReady(w http.ResponseWriter) bool {
	if h.mtproto == nil {
		writeError(w, http.StatusServiceUnavailable, "mtproto not available")

		return false
	}

	return true
}

// mtprotoConfig builds the proxy config from the persisted settings, so Enable
// and Rebuild always agree about what is being served.
func (h *Handler) mtprotoConfig() mtproto.Config {
	s := h.settings.Get().Mtproto

	return mtproto.Config{
		Listen:             s.Listen,
		MaskingDomain:      s.MaskingDomain,
		Concurrency:        uint(s.Concurrency),
		IdleTimeout:        time.Duration(s.IdleTimeoutSec) * time.Second,
		PreferIP:           s.PreferIP,
		DomainFrontingPort: uint(s.DomainFrontingPort),
	}
}

// mtprotoPublic resolves the host and port that go into tg:// links: the
// [mtproto] overrides when set, otherwise the panel's own public address — the
// same fallback subscription URLs use.
func (h *Handler) mtprotoPublic() (string, int) {
	s := h.settings.Get()

	host := s.Mtproto.PublicHost
	if host == "" {
		host = s.Server.PublicHost
	}

	port := s.Mtproto.PublicPort
	if port == 0 {
		// The listen port is the honest fallback: with nothing in front, that
		// is exactly what clients dial.
		if _, p, err := net.SplitHostPort(s.Mtproto.Listen); err == nil {
			port, _ = strconv.Atoi(p)
		}
	}

	return host, port
}

// mtprotoRebuild reapplies the roster to a running proxy and maps the failure
// onto a status code. A stopped proxy is not an error: editing the roster
// before enabling is normal.
func (h *Handler) mtprotoRebuild(w http.ResponseWriter) bool {
	if err := h.mtproto.Rebuild(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return false
	}

	return true
}

// ListMtprotoClients returns the roster. PROTECTED.
func (h *Handler) ListMtprotoClients(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	online := map[string]bool{}
	if es := h.mtproto.Events(); es != nil {
		for _, c := range es.Connections() {
			online[c.Client] = true
		}
	}

	rows := make([]mtprotoClientRow, 0)

	for _, c := range h.mtproto.Store().List() {
		rows = append(rows, mtprotoClientRow{
			Name:      c.Name,
			Enabled:   c.Enabled,
			CreatedAt: c.CreatedAt,
			ExpiresAt: c.ExpiresAt,
			Online:    online[c.Name],
		})
	}

	writeSuccess(w, rows)
}

// CreateMtprotoClient issues a new client and secret. PROTECTED.
func (h *Handler) CreateMtprotoClient(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	var body struct {
		Name      string `json:"name"`
		ExpiresAt int64  `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")

		return
	}

	// Trimmed, or two rows that look identical in the roster would be distinct
	// keys in the store.
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")

		return
	}

	if _, exists := h.mtproto.Store().Get(name); exists {
		// Replacing would silently revoke the link the existing client holds.
		writeError(w, http.StatusConflict, "a client with that name already exists")

		return
	}

	secret, err := mtproto.GenerateSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot generate a secret")

		return
	}

	client := mtproto.Client{
		Name:      name,
		Secret:    secret,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: body.ExpiresAt,
	}

	if err := h.mtproto.Store().Put(client); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	if !h.mtprotoRebuild(w) {
		return
	}

	writeSuccess(w, map[string]any{"name": client.Name})
}

// DeleteMtprotoClient revokes a client. PROTECTED.
func (h *Handler) DeleteMtprotoClient(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	name := chi.URLParam(r, "name")

	if _, ok := h.mtproto.Store().Get(name); !ok {
		writeError(w, http.StatusNotFound, "no such client")

		return
	}

	if err := h.mtproto.Store().Delete(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Deleting the last client stops the proxy, which Rebuild reports as an
	// error. That is the correct outcome, not a failed delete: the client is
	// gone and its secret is no longer served.
	if err := h.mtproto.Rebuild(); err != nil && !errors.Is(err, mtproto.ErrNoActiveClients) {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeSuccess(w, map[string]any{"deleted": name})
}

// UpdateMtprotoClient patches the fields an admin can change without reissuing
// a secret. Omitted fields are left alone. PROTECTED.
func (h *Handler) UpdateMtprotoClient(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	name := chi.URLParam(r, "name")

	client, ok := h.mtproto.Store().Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "no such client")

		return
	}

	// Pointers so an omitted field is distinguishable from a zero one: a PATCH
	// that silently cleared expires_at would quietly extend access.
	var body struct {
		Enabled   *bool  `json:"enabled"`
		ExpiresAt *int64 `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")

		return
	}

	if body.Enabled != nil {
		client.Enabled = *body.Enabled
	}

	if body.ExpiresAt != nil {
		client.ExpiresAt = *body.ExpiresAt
	}

	if err := h.mtproto.Store().Put(client); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Disabling the last enabled client stops the proxy, same as a delete.
	if err := h.mtproto.Rebuild(); err != nil && !errors.Is(err, mtproto.ErrNoActiveClients) {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeSuccess(w, map[string]any{"name": name})
}

// RotateMtprotoClient issues a fresh secret, invalidating the old link.
// PROTECTED.
func (h *Handler) RotateMtprotoClient(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	name := chi.URLParam(r, "name")

	client, ok := h.mtproto.Store().Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "no such client")

		return
	}

	secret, err := mtproto.GenerateSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot generate a secret")

		return
	}

	client.Secret = secret

	if err := h.mtproto.Store().Put(client); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	if !h.mtprotoRebuild(w) {
		return
	}

	writeSuccess(w, map[string]any{"name": name})
}

// GetMtprotoStatus returns the proxy status alongside its settings, so the page
// renders in one round trip. PROTECTED.
func (h *Handler) GetMtprotoStatus(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	host, port := h.mtprotoPublic()
	settings := h.settings.Get().Mtproto

	writeSuccess(w, map[string]any{
		"status":   h.mtproto.Status(),
		"settings": settings,
		// Resolved rather than raw, so the page can grey out the link actions
		// without duplicating the fallback logic.
		"public_host":    host,
		"public_port":    port,
		"can_issue_link": mtproto.CanIssueLink(settings.MaskingDomain, host),
		"read_only":      h.mtproto.Store().IsReadOnly(),
	})
}

// UpdateMtprotoSettings writes the [mtproto] block. PROTECTED.
func (h *Handler) UpdateMtprotoSettings(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	// Only the fields present in the body are applied, so the page can save one
	// section without echoing the rest back.
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")

		return
	}

	allowed := map[string]bool{
		"listen": true, "masking_domain": true, "public_host": true,
		"public_port": true, "concurrency": true, "idle_timeout_sec": true,
		"prefer_ip": true, "domain_fronting_port": true,
	}

	updates := map[string]any{}

	for key, value := range body {
		if !allowed[key] {
			// enabled is deliberately excluded: it is owned by
			// /enable and /disable, which also start and stop the listener.
			writeError(w, http.StatusBadRequest, "unknown setting: "+key)

			return
		}

		updates["mtproto."+key] = value
	}

	if err := h.settings.Update(updates); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	if err := h.settings.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	// A running proxy has to pick up the new listen address or masking domain;
	// a stopped one has nothing to do.
	if h.mtproto.Status().Running {
		if err := h.mtproto.Rebuild(); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())

			return
		}
	}

	writeSuccess(w, h.settings.Get().Mtproto)
}

// EnableMtproto starts the proxy and persists the flag. PROTECTED.
func (h *Handler) EnableMtproto(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	if err := h.mtproto.Start(h.mtprotoConfig()); err != nil {
		// The two "you forgot something" cases are a 400 naming the field;
		// anything else is genuinely broken.
		switch {
		case errors.Is(err, mtproto.ErrNoActiveClients),
			errors.Is(err, mtproto.ErrNoMaskingDomain):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, mtproto.ErrAlreadyRunning):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}

		return
	}

	if err := h.persistMtprotoEnabled(true); err != nil {
		// Best effort: the proxy is up, and failing the request would suggest
		// otherwise. It just will not survive a restart.
		log.Printf("mtproto: cannot persist enabled=true: %v", err)
	}

	writeSuccess(w, h.mtproto.Status())
}

// DisableMtproto stops the proxy and clears the flag. PROTECTED.
func (h *Handler) DisableMtproto(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	if err := h.mtproto.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	if err := h.persistMtprotoEnabled(false); err != nil {
		log.Printf("mtproto: cannot persist enabled=false: %v", err)
	}

	writeSuccess(w, h.mtproto.Status())
}

func (h *Handler) persistMtprotoEnabled(enabled bool) error {
	if err := h.settings.Update(map[string]any{"mtproto.enabled": enabled}); err != nil {
		return err
	}

	return h.settings.Save()
}

// GetMtprotoClientLink returns both link forms for one client.
//
// This is the only endpoint that discloses a secret, which is why it is fetched
// on demand rather than riding along with the roster listing. PROTECTED.
func (h *Handler) GetMtprotoClientLink(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	client, ok := h.mtproto.Store().Get(chi.URLParam(r, "name"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such client")

		return
	}

	domain := h.settings.Get().Mtproto.MaskingDomain
	host, port := h.mtprotoPublic()

	if !mtproto.CanIssueLink(domain, host) {
		// A link built from a missing piece is well-formed and fails silently
		// inside Telegram, so say what is missing instead of handing one over.
		writeError(w, http.StatusConflict,
			"set a masking domain and a public host before sharing links")

		return
	}

	writeSuccess(w, map[string]any{
		"tg":  mtproto.ProxyLink(host, port, client.Secret, domain),
		"web": mtproto.WebLink(host, port, client.Secret, domain),
	})
}

// GetMtprotoConnections returns the live matched streams. PROTECTED.
func (h *Handler) GetMtprotoConnections(w http.ResponseWriter, r *http.Request) {
	if !h.mtprotoReady(w) {
		return
	}

	conns := []mtproto.Connection{}
	if es := h.mtproto.Events(); es != nil {
		conns = es.Connections()
	}

	writeSuccess(w, conns)
}

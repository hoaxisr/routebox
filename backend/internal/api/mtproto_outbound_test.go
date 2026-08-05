package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/config"
	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/process"
	"routebox/backend/internal/settings"
)

// newMtprotoRoutingHandler is newMtprotoTestHandler plus a real config manager,
// which is what the outbound setting actually writes to.
func newMtprotoRoutingHandler(t *testing.T) (*Handler, http.Handler, string) {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	body := `{"log":{"level":"info"},
		"outbounds":[{"type":"direct","tag":"direct"}],
		"endpoints":[{"type":"wireguard","tag":"warp"}]}`

	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgMgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	settingsMgr, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		mtproto:  mtproto.NewManager(mtproto.NewStore("")),
		settings: settingsMgr,
		config:   cfgMgr,
	}
	// Nothing is running, so a sync never reaches Reload — which is what lets
	// these tests exercise the config side without a process manager.
	h.statusSource = func() process.Status { return process.Status{Running: false} }

	r := chi.NewRouter()
	r.Route("/api/mtproto", func(r chi.Router) {
		r.Get("/", h.GetMtprotoStatus)
		r.Put("/", h.UpdateMtprotoSettings)
	})

	return h, r, cfgPath
}

func managedSocksInbound(t *testing.T, m *config.Manager) map[string]interface{} {
	t.Helper()

	ib, _ := m.GetInbound(config.ManagedMtprotoSocksTag)

	return ib
}

func TestUpdateMtprotoSettingsWritesTheRouting(t *testing.T) {
	h, r, _ := newMtprotoRoutingHandler(t)

	rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto/", `{"outbound":"warp"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT outbound: %d %s", rec.Code, rec.Body)
	}

	if got := h.settings.Get().Mtproto.Outbound; got != "warp" {
		t.Errorf("persisted outbound = %q, want warp", got)
	}

	ib := managedSocksInbound(t, h.config)
	if ib == nil {
		t.Fatal("the managed SOCKS inbound was not written to the config")
	}

	if ib["listen"] != "127.0.0.1" {
		t.Errorf("listen = %v, want 127.0.0.1", ib["listen"])
	}

	// The proxy must now be told to dial it — a rule with nothing using it would
	// leave Telegram going out directly while the page claims otherwise.
	if got := h.mtprotoConfig().SocksProxy; got != "127.0.0.1:1080" {
		t.Errorf("SocksProxy = %q, want 127.0.0.1:1080", got)
	}

	// ...and switching back to direct must take it all away again.
	rec = mtprotoReq(t, r, http.MethodPut, "/api/mtproto/", `{"outbound":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT direct: %d %s", rec.Code, rec.Body)
	}

	if managedSocksInbound(t, h.config) != nil {
		t.Error("the managed inbound survived a switch back to direct")
	}

	if got := h.mtprotoConfig().SocksProxy; got != "" {
		t.Errorf("SocksProxy = %q, want empty for a direct exit", got)
	}
}

func TestUpdateMtprotoSettingsHonoursTheSocksPort(t *testing.T) {
	h, r, _ := newMtprotoRoutingHandler(t)

	rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto/",
		`{"outbound":"direct","socks_port":1091}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}

	ib := managedSocksInbound(t, h.config)
	if ib == nil {
		t.Fatal("the managed SOCKS inbound was not written")
	}

	if port, _ := ib["listen_port"].(float64); port != 1091 {
		t.Errorf("listen_port = %v, want 1091", ib["listen_port"])
	}

	if got := h.mtprotoConfig().SocksProxy; got != "127.0.0.1:1091" {
		t.Errorf("SocksProxy = %q, want 127.0.0.1:1091", got)
	}
}

// A rule naming a tag that is not there makes sing-box reject the whole config
// on its next reload — so this has to be refused before anything is persisted,
// not discovered when the VPN fails to come back up.
func TestUpdateMtprotoSettingsRejectsAnUnknownOutbound(t *testing.T) {
	h, r, _ := newMtprotoRoutingHandler(t)

	rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto/", `{"outbound":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400: %s", rec.Code, rec.Body)
	}

	if !strings.Contains(rec.Body.String(), "nope") {
		t.Errorf("the error should name the tag: %s", rec.Body)
	}

	if got := h.settings.Get().Mtproto.Outbound; got != "" {
		t.Errorf("a rejected outbound was persisted anyway: %q", got)
	}

	if managedSocksInbound(t, h.config) != nil {
		t.Error("a rejected outbound wrote an inbound anyway")
	}
}

// SyncMtprotoSocksActive silently defers while a draft is pending, so a save
// that reached it would report success and route nothing. Refuse up front.
func TestUpdateMtprotoSettingsRefusesWhileADraftIsPending(t *testing.T) {
	h, r, _ := newMtprotoRoutingHandler(t)

	if err := h.config.CreateOutbound(map[string]interface{}{"type": "direct", "tag": "staged"}); err != nil {
		t.Fatalf("CreateOutbound: %v", err)
	}

	rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto/", `{"outbound":"warp"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409: %s", rec.Code, rec.Body)
	}

	if got := h.settings.Get().Mtproto.Outbound; got != "" {
		t.Errorf("the outbound was persisted despite the pending draft: %q", got)
	}

	// A save that does not touch the outbound is unaffected — the draft only
	// blocks the setting that has to reach the active config.
	rec = mtprotoReq(t, r, http.MethodPut, "/api/mtproto/", `{"concurrency":2048}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unrelated save: %d %s", rec.Code, rec.Body)
	}
}

// A port already taken by another inbound cannot be written — and the save must
// leave the settings describing what is actually running, not a routing that was
// never created. Otherwise the proxy would dial a listener that does not exist.
func TestUpdateMtprotoSettingsRollsBackWhenTheRoutingCannotBeWritten(t *testing.T) {
	h, r, _ := newMtprotoRoutingHandler(t)

	// Occupy the port the Telegram proxy is about to ask for.
	if err := h.config.CreateInbound(map[string]interface{}{
		"type": "socks", "tag": "local-socks", "listen": "127.0.0.1", "listen_port": float64(1080),
	}); err != nil {
		t.Fatalf("CreateInbound: %v", err)
	}

	if err := h.config.ApplyDraft(); err != nil {
		t.Fatalf("ApplyDraft: %v", err)
	}

	rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto/", `{"outbound":"warp"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("want a failure for a colliding port, got 200: %s", rec.Body)
	}

	// Naming the port proves the request got past the tag and draft checks and
	// actually failed inside the sync — which is the path the rollback guards.
	if !strings.Contains(rec.Body.String(), "1080") {
		t.Fatalf("the error should name the colliding port: %s", rec.Body)
	}

	if got := h.settings.Get().Mtproto.Outbound; got != "" {
		t.Errorf("outbound = %q after a failed save, want it rolled back to empty", got)
	}

	if got := h.mtprotoConfig().SocksProxy; got != "" {
		t.Errorf("SocksProxy = %q, want empty — the proxy must not dial a listener that was never created", got)
	}
}

func TestGetMtprotoStatusOffersTheRoutableTags(t *testing.T) {
	_, r, _ := newMtprotoRoutingHandler(t)

	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/", "")

	var body struct {
		Outbounds []config.RoutableTag `json:"outbounds"`
	}

	decodeMtprotoData(t, rec, &body)

	if len(body.Outbounds) != 2 {
		t.Fatalf("got %d tags, want the outbound and the endpoint: %#v", len(body.Outbounds), body.Outbounds)
	}

	if body.Outbounds[0].Tag != "direct" || body.Outbounds[0].Kind != "outbound" {
		t.Errorf("first tag = %#v, want direct/outbound", body.Outbounds[0])
	}

	if body.Outbounds[1].Tag != "warp" || body.Outbounds[1].Kind != "endpoint" {
		t.Errorf("second tag = %#v, want warp/endpoint", body.Outbounds[1])
	}
}

// Router mode has no sing-box config; the page still has to render.
func TestGetMtprotoStatusWithoutAConfigManager(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/", "")

	var body struct {
		Outbounds []config.RoutableTag `json:"outbounds"`
	}

	decodeMtprotoData(t, rec, &body)

	if body.Outbounds == nil {
		t.Error("outbounds must serialise as [] rather than null, or the picker cannot iterate it")
	}
}

func TestSocksProxyAddr(t *testing.T) {
	if got := mtproto.SocksProxyAddr("", 1080); got != "" {
		t.Errorf("no outbound -> %q, want empty (direct)", got)
	}

	if got := mtproto.SocksProxyAddr("warp", 0); got != "127.0.0.1:1080" {
		t.Errorf("unset port -> %q, want the default 1080", got)
	}

	if got := mtproto.SocksProxyAddr("warp", 1091); got != "127.0.0.1:1091" {
		t.Errorf("got %q, want 127.0.0.1:1091", got)
	}
}

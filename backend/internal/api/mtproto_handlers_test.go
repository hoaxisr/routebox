package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/settings"
)

const testSecret = "00112233445566778899aabbccddeeff"

// newMtprotoTestHandler builds a handler and the router main.go mounts, so the
// tests pin route precedence as well as the handlers themselves.
func newMtprotoTestHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()

	settingsMgr, err := settings.NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		mtproto:  mtproto.NewManager(mtproto.NewStore("")),
		settings: settingsMgr,
	}

	r := chi.NewRouter()
	r.Route("/api/mtproto", func(r chi.Router) {
		r.Get("/", h.GetMtprotoStatus)
		r.Put("/", h.UpdateMtprotoSettings)
		r.Post("/enable", h.EnableMtproto)
		r.Post("/disable", h.DisableMtproto)
		r.Get("/connections", h.GetMtprotoConnections)
		r.Get("/clients", h.ListMtprotoClients)
		r.Post("/clients", h.CreateMtprotoClient)
		r.Delete("/clients/{name}", h.DeleteMtprotoClient)
		r.Patch("/clients/{name}", h.UpdateMtprotoClient)
		r.Get("/clients/{name}/link", h.GetMtprotoClientLink)
		r.Post("/clients/{name}/rotate", h.RotateMtprotoClient)
	})

	return h, r
}

func mtprotoReq(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, reader))

	return rec
}

// decodeData unwraps the {success, data} envelope every handler here uses.
func decodeMtprotoData(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()

	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not the {success,data} envelope: %v\nbody: %s", err, rec.Body)
	}

	if !env.Success {
		t.Fatalf("success=false: %s", env.Error)
	}

	if into != nil {
		if err := json.Unmarshal(env.Data, into); err != nil {
			t.Fatalf("cannot decode data: %v\nbody: %s", err, rec.Body)
		}
	}
}

func TestCreateClientIssuesASecret(t *testing.T) {
	h, r := newMtprotoTestHandler(t)

	rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients", `{"name":"alice"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	c, ok := h.mtproto.Store().Get("alice")
	if !ok {
		t.Fatal("alice was not stored")
	}

	if len(c.Secret) != 32 {
		t.Errorf("secret = %q, want 32 hex chars", c.Secret)
	}

	if !c.Enabled {
		t.Error("a freshly created client should be enabled")
	}

	if c.CreatedAt == 0 {
		t.Error("CreatedAt was not stamped")
	}
}

func TestCreateClientTrimsTheName(t *testing.T) {
	h, r := newMtprotoTestHandler(t)

	mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients", `{"name":"  alice  "}`)

	if _, ok := h.mtproto.Store().Get("alice"); !ok {
		t.Error("the name was not trimmed; leading space would make two lookalike rows")
	}
}

func TestCreateClientRejectsADuplicateName(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret}); err != nil {
		t.Fatal(err)
	}

	rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients", `{"name":"alice"}`)

	// Replacing silently would revoke the link the existing client holds.
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestCreateClientRejectsABlankName(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{}`} {
		if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestCreateClientRejectsInvalidJSON(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients", `{not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestListDoesNotLeakRawSecrets(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients", "")

	// The page polls this. A credential riding on every poll is a credential in
	// every log and proxy buffer; the link endpoint serves it on demand.
	if strings.Contains(rec.Body.String(), testSecret) {
		t.Errorf("the raw secret appears in the roster listing: %s", rec.Body)
	}
}

func TestListReportsTheRoster(t *testing.T) {
	h, r := newMtprotoTestHandler(t)

	for _, c := range []mtproto.Client{
		{Name: "zoe", Secret: testSecret, Enabled: true},
		{Name: "adam", Secret: testSecret, Enabled: false, ExpiresAt: 99},
	} {
		if err := h.mtproto.Store().Put(c); err != nil {
			t.Fatal(err)
		}
	}

	var rows []struct {
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		ExpiresAt int64  `json:"expires_at"`
		Online    bool   `json:"online"`
	}

	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients", ""), &rows)

	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2", rows)
	}

	if rows[0].Name != "adam" || rows[1].Name != "zoe" {
		t.Errorf("rows = %+v, want them sorted by name", rows)
	}

	if rows[0].Enabled || rows[0].ExpiresAt != 99 {
		t.Errorf("adam = %+v, want disabled with expiry 99", rows[0])
	}
}

func TestListIsAnEmptyArrayNotNull(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients", "")

	// A null would make the roster table render an error instead of "no clients".
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("body = %s, want data to be []", rec.Body)
	}
}

func TestRotateChangesTheSecret(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients/alice/rotate", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	c, _ := h.mtproto.Store().Get("alice")
	if c.Secret == testSecret {
		t.Error("rotate did not change the secret")
	}
}

func TestRotatePreservesEverythingElse(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{
		Name: "alice", Secret: testSecret, Enabled: true, CreatedAt: 7, ExpiresAt: 900,
	}); err != nil {
		t.Fatal(err)
	}

	mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients/alice/rotate", "")

	c, _ := h.mtproto.Store().Get("alice")
	if !c.Enabled || c.CreatedAt != 7 || c.ExpiresAt != 900 {
		t.Errorf("got %+v, want only the secret to have changed", c)
	}
}

func TestRotateAnUnknownClientIs404(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/clients/nobody/rotate", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteRemovesTheClient(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret}); err != nil {
		t.Fatal(err)
	}

	if rec := mtprotoReq(t, r, http.MethodDelete, "/api/mtproto/clients/alice", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	if _, ok := h.mtproto.Store().Get("alice"); ok {
		t.Error("alice is still in the roster")
	}
}

func TestDeleteAnUnknownClientIs404(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodDelete, "/api/mtproto/clients/nobody", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPatchTogglesEnabledAndExpiry(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	rec := mtprotoReq(t, r, http.MethodPatch, "/api/mtproto/clients/alice", `{"enabled":false,"expires_at":1234}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	c, _ := h.mtproto.Store().Get("alice")
	if c.Enabled || c.ExpiresAt != 1234 {
		t.Errorf("got %+v, want disabled with expiry 1234", c)
	}
}

func TestPatchLeavesOmittedFieldsAlone(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{
		Name: "alice", Secret: testSecret, Enabled: true, ExpiresAt: 555,
	}); err != nil {
		t.Fatal(err)
	}

	mtprotoReq(t, r, http.MethodPatch, "/api/mtproto/clients/alice", `{"enabled":false}`)

	// A PATCH that silently cleared the expiry would quietly extend access.
	c, _ := h.mtproto.Store().Get("alice")
	if c.ExpiresAt != 555 {
		t.Errorf("ExpiresAt = %d, want 555 left untouched", c.ExpiresAt)
	}

	if c.Secret != testSecret {
		t.Errorf("Secret changed on a PATCH: %q", c.Secret)
	}
}

func TestPatchAnUnknownClientIs404(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodPatch, "/api/mtproto/clients/nobody", `{"enabled":true}`); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestStatusReportsStoppedAndCarriesSettings(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	var body struct {
		Status struct {
			Running bool `json:"running"`
		} `json:"status"`
		Settings struct {
			Listen string `json:"listen"`
		} `json:"settings"`
	}

	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto", ""), &body)

	if body.Status.Running {
		t.Error("running = true for a manager that was never started")
	}

	if body.Settings.Listen == "" {
		t.Error("the settings block must come along, so the page renders in one round trip")
	}
}

func TestLinkReturnsBothForms(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := h.settings.Update(map[string]any{
		"mtproto.masking_domain": "example.com",
		"mtproto.public_host":    "panel.example.com",
		"mtproto.public_port":    int64(443),
	}); err != nil {
		t.Fatal(err)
	}

	var body struct {
		TG  string `json:"tg"`
		Web string `json:"web"`
	}

	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/alice/link", ""), &body)

	if !strings.HasPrefix(body.TG, "tg://proxy?") || !strings.HasPrefix(body.Web, "https://t.me/proxy?") {
		t.Errorf("got %+v, want both link forms", body)
	}

	if !strings.Contains(body.TG, "panel.example.com") || !strings.Contains(body.TG, "port=443") {
		t.Errorf("tg link = %q, want the public host and port, not the listen address", body.TG)
	}
}

func TestLinkRefusesWithoutAMaskingDomain(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := h.settings.Update(map[string]any{"mtproto.public_host": "panel.example.com"}); err != nil {
		t.Fatal(err)
	}

	// A link missing the domain looks fine and fails silently inside Telegram.
	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/alice/link", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestLinkForAnUnknownClientIs404(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/nobody/link", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestConnectionsIsAnArrayWhenIdle(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/connections", "")
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("body = %s, want data to be []", rec.Body)
	}
}

func TestEnableWithoutClientsExplainsWhy(t *testing.T) {
	h, r := newMtprotoTestHandler(t)

	if err := h.settings.Update(map[string]any{"mtproto.masking_domain": "example.com"}); err != nil {
		t.Fatal(err)
	}

	rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/enable", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body)
	}

	if !strings.Contains(rec.Body.String(), "client") {
		t.Errorf("body = %s, want it to name the missing piece", rec.Body)
	}
}

func TestEnableWithoutAMaskingDomainExplainsWhy(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/enable", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body)
	}

	if !strings.Contains(rec.Body.String(), "masking domain") {
		t.Errorf("body = %s, want it to name the missing setting", rec.Body)
	}
}

func TestEnableThenDisable(t *testing.T) {
	h, r := newMtprotoTestHandler(t)
	if err := h.mtproto.Store().Put(mtproto.Client{Name: "alice", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := h.settings.Update(map[string]any{
		"mtproto.masking_domain": "example.com",
		"mtproto.listen":         "127.0.0.1:0",
	}); err != nil {
		t.Fatal(err)
	}

	if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/enable", ""); rec.Code != http.StatusOK {
		t.Fatalf("enable: status = %d, body = %s", rec.Code, rec.Body)
	}

	if !h.mtproto.Status().Running {
		t.Fatal("the proxy is not running after enable")
	}

	// Enable must persist, or a restart would silently drop the proxy.
	if !h.settings.Get().Mtproto.Enabled {
		t.Error("mtproto.enabled was not persisted")
	}

	if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/disable", ""); rec.Code != http.StatusOK {
		t.Fatalf("disable: status = %d, body = %s", rec.Code, rec.Body)
	}

	if h.mtproto.Status().Running {
		t.Error("the proxy is still running after disable")
	}

	if h.settings.Get().Mtproto.Enabled {
		t.Error("mtproto.enabled was not cleared")
	}
}

func TestDisableOnAStoppedProxyIsFine(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodPost, "/api/mtproto/disable", ""); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestUpdateSettingsPersists(t *testing.T) {
	h, r := newMtprotoTestHandler(t)

	rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto",
		`{"masking_domain":"storage.googleapis.com","listen":"0.0.0.0:9443","public_port":443}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	got := h.settings.Get().Mtproto
	if got.MaskingDomain != "storage.googleapis.com" || got.Listen != "0.0.0.0:9443" || got.PublicPort != 443 {
		t.Errorf("got %+v, want the submitted values", got)
	}
}

func TestUpdateSettingsRejectsInvalidJSON(t *testing.T) {
	_, r := newMtprotoTestHandler(t)

	if rec := mtprotoReq(t, r, http.MethodPut, "/api/mtproto", `{nope`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestEveryRouteReports503WithoutAManager(t *testing.T) {
	h := &Handler{}

	r := chi.NewRouter()
	r.Route("/api/mtproto", func(r chi.Router) {
		r.Get("/", h.GetMtprotoStatus)
		r.Put("/", h.UpdateMtprotoSettings)
		r.Post("/enable", h.EnableMtproto)
		r.Post("/disable", h.DisableMtproto)
		r.Get("/connections", h.GetMtprotoConnections)
		r.Get("/clients", h.ListMtprotoClients)
		r.Post("/clients", h.CreateMtprotoClient)
		r.Delete("/clients/{name}", h.DeleteMtprotoClient)
		r.Patch("/clients/{name}", h.UpdateMtprotoClient)
		r.Get("/clients/{name}/link", h.GetMtprotoClientLink)
		r.Post("/clients/{name}/rotate", h.RotateMtprotoClient)
	})

	// Router mode leaves the manager nil; every route must say so rather than
	// panic on a nil dereference.
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/mtproto"},
		{http.MethodPut, "/api/mtproto"},
		{http.MethodPost, "/api/mtproto/enable"},
		{http.MethodPost, "/api/mtproto/disable"},
		{http.MethodGet, "/api/mtproto/connections"},
		{http.MethodGet, "/api/mtproto/clients"},
		{http.MethodPost, "/api/mtproto/clients"},
		{http.MethodDelete, "/api/mtproto/clients/alice"},
		{http.MethodPatch, "/api/mtproto/clients/alice"},
		{http.MethodGet, "/api/mtproto/clients/alice/link"},
		{http.MethodPost, "/api/mtproto/clients/alice/rotate"},
	} {
		rec := mtprotoReq(t, r, tt.method, tt.path, `{}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status = %d, want 503", tt.method, tt.path, rec.Code)
		}
	}
}

package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/config"
	"routebox/backend/internal/subscriptions"
)

func subB64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func newSubsHandler(t *testing.T) (*Handler, *config.Manager) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"tag":"direct","type":"direct"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfgMgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	subsMgr := subscriptions.NewManager(filepath.Join(dir, "subscriptions.toml"))
	h := NewHandler(cfgMgr, nil, "", nil, nil, nil, nil)
	h.SetSubscriptions(subsMgr, func(s subscriptions.Subscription) (int, int, error) {
		return subscriptions.Refresh(s, cfgMgr)
	})
	return h, cfgMgr
}

func subsRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/subscriptions", func(r chi.Router) {
		r.Get("/", h.ListSubscriptions)
		r.Post("/", h.CreateSubscription)
		r.Put("/{id}", h.UpdateSubscription)
		r.Delete("/{id}", h.DeleteSubscription)
		r.Post("/{id}/refresh", h.RefreshSubscription)
	})
	return r
}

func TestCreateSubscriptionMergesDraft(t *testing.T) {
	body := subB64("ss://YWVzLTEyOC1nY206cGFzcw==@1.2.3.4:8388#Tokyo\nvless://uuid@5.6.7.8:443#Osaka")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer srv.Close()
	h, cfgMgr := newSubsHandler(t)
	r := subsRouter(h)
	payload, _ := json.Marshal(map[string]interface{}{"name": "Home", "url": srv.URL, "interval_hrs": 12})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/subscriptions", strings.NewReader(string(payload))))
	if rec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	if g, ok := cfgMgr.GetOutbound("Home"); !ok || g["type"] != "urltest" {
		t.Fatalf("group missing/wrong: %+v ok=%v", g, ok)
	}
	prefixed := 0
	for _, ob := range cfgMgr.ListOutbounds() {
		if tag, _ := ob["tag"].(string); strings.HasPrefix(tag, "Home · ") {
			prefixed++
		}
	}
	if prefixed != 2 {
		t.Fatalf("expected 2 prefixed nodes, got %d", prefixed)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/subscriptions", strings.NewReader(string(payload))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate create status=%d, want 400", rec.Code)
	}
}

func TestListAndRefreshAndDelete(t *testing.T) {
	body := subB64("ss://YWVzLTEyOC1nY206cGFzcw==@1.2.3.4:8388#Tokyo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer srv.Close()
	h, cfgMgr := newSubsHandler(t)
	r := subsRouter(h)
	payload, _ := json.Marshal(map[string]interface{}{"name": "Home", "url": srv.URL, "interval_hrs": 6})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/subscriptions", strings.NewReader(string(payload))))
	if rec.Code != http.StatusOK {
		t.Fatalf("create failed: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/subscriptions", nil))
	var listResp struct {
		Data []subscriptions.Subscription `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&listResp)
	if len(listResp.Data) != 1 || listResp.Data[0].NodeCount != 1 {
		t.Fatalf("list wrong: %+v", listResp.Data)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/subscriptions/home/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/subscriptions/home", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d", rec.Code)
	}
	if _, ok := cfgMgr.GetOutbound("Home"); ok {
		t.Fatal("group should be gone after delete")
	}
	for _, ob := range cfgMgr.ListOutbounds() {
		if tag, _ := ob["tag"].(string); strings.HasPrefix(tag, "Home · ") {
			t.Fatalf("prefixed node survived delete: %s", tag)
		}
	}
}

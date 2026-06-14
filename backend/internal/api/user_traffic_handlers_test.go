package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/traffic"
	"routebox/backend/internal/users"
)

func openAPITrafficStore(t *testing.T) *traffic.Store {
	t.Helper()
	s, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func chiNewRouterUserTraffic(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/users/{id}/traffic", h.GetUserTraffic)
	return r
}

func TestGetUserTraffic_SumsAndHistory(t *testing.T) {
	store := openAPITrafficStore(t)
	// Two distinct buckets inside the ?range=24h window (now-relative).
	b1 := time.Now().Unix() - 600
	b2 := time.Now().Unix() - 300
	_ = store.UpsertUser(b1, "alice", 100, 200)
	_ = store.UpsertUser(b2, "alice", 50, 60)

	reg := users.NewManager("")
	_ = reg.Put(&users.PanelUser{ID: "u1", Name: "alice",
		Bindings: []users.Binding{{InboundTag: "in", Credential: "c", Protocol: "vless", Name: "alice"}}})

	h := &Handler{traffic: store, panelUsers: reg}
	r := chiNewRouterUserTraffic(h)

	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/traffic?range=24h", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Upload   int64 `json:"upload"`
			Download int64 `json:"download"`
			History  []struct {
				Ts       int64 `json:"ts"`
				Upload   int64 `json:"upload"`
				Download int64 `json:"download"`
			} `json:"history"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Upload != 150 || resp.Data.Download != 260 {
		t.Errorf("totals = %d/%d, want 150/260", resp.Data.Upload, resp.Data.Download)
	}
	if len(resp.Data.History) != 2 {
		t.Errorf("history len = %d, want 2", len(resp.Data.History))
	}
}

func TestGetUserTraffic_MultiNameMerge(t *testing.T) {
	store := openAPITrafficStore(t)
	// Two distinct buckets inside the ?range=24h window (now-relative).
	early := time.Now().Unix() - 600
	late := time.Now().Unix() - 300
	// Same (early) bucket, two different names → must merge per-bucket.
	_ = store.UpsertUser(early, "alice", 100, 200)
	_ = store.UpsertUser(early, "alice-phone", 1, 2)
	// Distinct (late) bucket under the binding name.
	_ = store.UpsertUser(late, "alice-phone", 50, 60)

	reg := users.NewManager("")
	// Panel Name ("primary") differs from both binding names.
	_ = reg.Put(&users.PanelUser{ID: "u1", Name: "alice",
		Bindings: []users.Binding{
			{InboundTag: "in", Credential: "c1", Protocol: "vless", Name: "alice"},
			{InboundTag: "in", Credential: "c2", Protocol: "vless", Name: "alice-phone"},
		}})

	h := &Handler{traffic: store, panelUsers: reg}
	r := chiNewRouterUserTraffic(h)

	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/traffic?range=24h", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Upload   int64 `json:"upload"`
			Download int64 `json:"download"`
			History  []struct {
				Ts       int64 `json:"ts"`
				Upload   int64 `json:"upload"`
				Download int64 `json:"download"`
			} `json:"history"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// 100+1+50 up, 200+2+60 down across both names.
	if resp.Data.Upload != 151 || resp.Data.Download != 262 {
		t.Errorf("totals = %d/%d, want 151/262", resp.Data.Upload, resp.Data.Download)
	}
	// Two distinct buckets (early merged, late) ascending.
	if len(resp.Data.History) != 2 {
		t.Fatalf("history len = %d, want 2", len(resp.Data.History))
	}
	if resp.Data.History[0].Ts != early || resp.Data.History[1].Ts != late {
		t.Errorf("ts order = %d,%d, want %d,%d", resp.Data.History[0].Ts, resp.Data.History[1].Ts, early, late)
	}
	// Early bucket merges both names: up 100+1=101, down 200+2=202.
	if resp.Data.History[0].Upload != 101 || resp.Data.History[0].Download != 202 {
		t.Errorf("early bucket = %d/%d, want 101/202", resp.Data.History[0].Upload, resp.Data.History[0].Download)
	}
}

func TestGetUserTraffic_UnknownUser404(t *testing.T) {
	store := openAPITrafficStore(t)
	h := &Handler{traffic: store, panelUsers: users.NewManager("")}
	r := chiNewRouterUserTraffic(h)
	req := httptest.NewRequest(http.MethodGet, "/api/users/ghost/traffic?range=24h", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestPanelUserNames_DedupesAndSkipsBlank(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "1", Name: "alice"})
	_ = mgr.Put(&users.PanelUser{ID: "2", Name: "bob"})
	_ = mgr.Put(&users.PanelUser{ID: "3", Name: ""}) // blank skipped

	got := panelUserNames(mgr)
	if len(got) != 2 {
		t.Fatalf("names = %v, want 2 (alice,bob)", got)
	}
	want := map[string]bool{"alice": true, "bob": true}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected name %q", n)
		}
	}
}

func TestPanelUserNames_NilManagerEmpty(t *testing.T) {
	if got := panelUserNames(nil); len(got) != 0 {
		t.Errorf("nil mgr → %v, want empty", got)
	}
}

func TestUserTrafficNames_NameUnionBindings(t *testing.T) {
	u := users.PanelUser{
		Name: "alice",
		Bindings: []users.Binding{
			{Name: "alice"},       // dup of Name
			{Name: "alice-phone"}, // extra
			{Name: ""},            // skipped
		},
	}
	got := userTrafficNames(u)
	if len(got) != 2 {
		t.Fatalf("names = %v, want [alice alice-phone]", got)
	}
	seen := map[string]bool{}
	for _, n := range got {
		seen[n] = true
	}
	if !seen["alice"] || !seen["alice-phone"] {
		t.Errorf("got %v, want alice + alice-phone", got)
	}
}

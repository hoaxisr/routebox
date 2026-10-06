package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/asnsets"
	"routebox/backend/internal/config"
	"routebox/backend/internal/util"
)

// newASNHandler wires a real config manager and an asnsets manager whose
// RIPEstat is a fake server: AS13335 has prefixes, everything else none.
//
// Layout under the returned dir: cfg/config.json (and its draft), asn.toml and
// asn/ — the config lives in its own subdirectory so a test can make the draft
// write fail without touching the ASN side.
func newASNHandler(t *testing.T) (*Handler, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "cfg")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"rule_set":[{"type":"inline","tag":"taken","rules":[]}]}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cm, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	ripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("resource") == "AS13335" {
			_, _ = w.Write([]byte(`{"status":"ok","data":{"holder":"Cloudflare","prefixes":[{"prefix":"1.1.1.0/24"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":{"holder":"","prefixes":[]}}`))
	}))
	t.Cleanup(ripe.Close)
	f := asnsets.NewFetcher()
	f.BaseURL, f.RetryDelay = ripe.URL, 0
	m := asnsets.NewManager(asnsets.NewStore(filepath.Join(dir, "asn.toml")), filepath.Join(dir, "asn"), f)
	h := &Handler{config: cm}
	h.SetASN(m)
	r := chi.NewRouter()
	r.Route("/api/route/rule-sets", func(r chi.Router) {
		r.Get("/asn", h.ListAsnSets)
		r.Post("/asn", h.CreateAsnSet)
		r.Put("/asn/{tag}", h.UpdateAsnSet)
		r.Post("/asn/{tag}/refresh", h.RefreshAsnSet)
	})
	return h, r, dir
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(b)))
	return rec
}

func TestCreateAsnSetAddsLocalRuleSetToDraft(t *testing.T) {
	h, r, dir := newASNHandler(t)
	rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"AS13335"}, "interval_hrs": 24})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var found map[string]interface{}
	for _, rs := range h.config.ListRuleSets() {
		if rs["tag"] == "cf" {
			found = rs
		}
	}
	want := filepath.Join(dir, "asn", "cf.json")
	if found == nil || found["type"] != "local" || found["format"] != "source" || found["path"] != want {
		t.Fatalf("draft rule set = %v", found)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
	// The entry is in the draft, not yet in the active config.
	for _, rs := range ruleSetsOf(h.config.GetActive()) {
		if rs["tag"] == "cf" {
			t.Fatal("rule set landed in the active config without apply")
		}
	}
}

func ruleSetsOf(cfg map[string]interface{}) []map[string]interface{} {
	route, _ := cfg["route"].(map[string]interface{})
	arr, _ := route["rule_set"].([]interface{})
	var out []map[string]interface{}
	for _, v := range arr {
		if rs, _ := v.(map[string]interface{}); rs != nil {
			out = append(out, rs)
		}
	}
	return out
}

func TestCreateAsnSetErrors(t *testing.T) {
	h, r, _ := newASNHandler(t)
	cases := []struct {
		body any
		code int
	}{
		{map[string]any{"tag": "taken", "asns": []string{"13335"}}, http.StatusBadRequest}, // tag used by another rule set
		{map[string]any{"tag": "x", "asns": []string{"AS-1"}}, http.StatusBadRequest},      // bad ASN
		{map[string]any{"tag": "../x", "asns": []string{"13335"}}, http.StatusBadRequest},  // unsafe tag
		{map[string]any{"tag": "y", "asns": []string{"64512"}}, http.StatusBadGateway},     // announces nothing
		{map[string]any{"tag": "z", "asns": []string{"13335"}, "interval_hrs": 7}, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", c.body); rec.Code != c.code {
			t.Errorf("%v: status %d, want %d (%s)", c.body, rec.Code, c.code, rec.Body)
		}
	}
	// A 502 for AS64512 must name the AS the way the brief fixes it.
	rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "y", "asns": []string{"64512"}})
	if !bytes.Contains(rec.Body.Bytes(), []byte("AS64512:")) {
		t.Errorf("502 body does not name the AS: %s", rec.Body)
	}
	if n := len(h.asn.List()); n != 0 {
		t.Fatalf("failed creates left %d entries", n)
	}
	if n := len(h.config.ListRuleSets()); n != 1 {
		t.Fatalf("failed creates touched the config: %d rule sets", n)
	}
	// Malformed JSON is a 400 too.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/route/rule-sets/asn", bytes.NewReader([]byte("{"))))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON: status %d", rec.Code)
	}
}

// TestCreateAsnSetReadoptsOrphan: a RouteBox restart discards the unapplied
// draft, so a set created but never applied is in the store with no config
// entry. POSTing the tag again must re-adopt it (200, draft entry back, ASNs
// updated) instead of answering "already exists" until Prune reaps it.
func TestCreateAsnSetReadoptsOrphan(t *testing.T) {
	h, r, dir := newASNHandler(t)
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}, "interval_hrs": 24}); rec.Code != http.StatusOK {
		t.Fatalf("create %d: %s", rec.Code, rec.Body)
	}
	// Simulate the restart: a fresh config manager on the same file drops the draft.
	cm, err := config.NewManager(filepath.Join(dir, "cfg", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	h.config = cm
	if n := len(h.config.ListRuleSets()); n != 1 {
		t.Fatalf("precondition: draft should be gone, got %d rule sets", n)
	}
	if _, ok := h.asn.Get("cf"); !ok {
		t.Fatal("precondition: ASN set should survive the restart")
	}

	rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335", "AS13335"}, "interval_hrs": 6})
	if rec.Code != http.StatusOK {
		t.Fatalf("re-adopt: status %d, want 200 (%s)", rec.Code, rec.Body)
	}
	var found map[string]interface{}
	for _, rs := range h.config.ListRuleSets() {
		if rs["tag"] == "cf" {
			found = rs
		}
	}
	want := filepath.Join(dir, "asn", "cf.json")
	if found == nil || found["type"] != "local" || found["format"] != "source" || found["path"] != want {
		t.Fatalf("draft rule set after re-adopt = %v", found)
	}
	e, _ := h.asn.Get("cf")
	if e.IntervalHrs != 6 || len(e.ASNs) != 1 || e.ASNs[0] != 13335 {
		t.Fatalf("entry not updated: %+v", e)
	}
	if n := len(h.asn.List()); n != 1 {
		t.Fatalf("re-adopt created a second entry: %d", n)
	}

	// Now the tag IS in the config: a third POST is a plain conflict.
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("existing set in config: status %d, want 400 (%s)", rec.Code, rec.Body)
	}
}

func TestCreateAsnSetRollsBackOnDraftFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	h, r, dir := newASNHandler(t)
	// The draft is written next to config.json (cfg/config.json.bak); make that
	// directory unwritable while asn.toml and asn/ (one level up) stay
	// writable, so Create succeeds and only the draft write fails.
	cfgDir := filepath.Join(dir, "cfg")
	if err := os.Chmod(cfgDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0755) })
	rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}})
	if rec.Code == http.StatusOK {
		t.Fatalf("expected failure, got 200")
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("read-only draft: status %d, want 409 (%s)", rec.Code, rec.Body)
	}
	if len(h.asn.List()) != 0 {
		t.Fatal("ASN entry not rolled back")
	}
	if _, err := os.Stat(filepath.Join(dir, "asn", "cf.json")); !os.IsNotExist(err) {
		t.Fatalf("prefix file not rolled back: %v", err)
	}
	if n := len(h.config.ListRuleSets()); n != 1 {
		t.Fatalf("draft kept a rule set after the failed write: %d", n)
	}
}

func TestUpdateAndRefreshAsnSet(t *testing.T) {
	_, r, _ := newASNHandler(t)
	_ = doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}})
	if rec := doJSON(t, r, "PUT", "/api/route/rule-sets/asn/cf", map[string]any{"asns": []string{"13335"}, "interval_hrs": 6}); rec.Code != http.StatusOK {
		t.Fatalf("update %d: %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, r, "PUT", "/api/route/rule-sets/asn/nope", map[string]any{"asns": []string{"13335"}}); rec.Code != http.StatusNotFound {
		t.Fatalf("update unknown: %d", rec.Code)
	}
	if rec := doJSON(t, r, "PUT", "/api/route/rule-sets/asn/cf", map[string]any{"asns": []string{"64512"}}); rec.Code != http.StatusBadGateway {
		t.Fatalf("update to an AS with no prefixes: %d, want 502", rec.Code)
	}
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn/cf/refresh", nil); rec.Code != http.StatusOK {
		t.Fatalf("refresh %d: %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn/nope/refresh", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("refresh unknown: %d", rec.Code)
	}
	rec := doJSON(t, r, "GET", "/api/route/rule-sets/asn", nil)
	var body struct {
		Data []asnsets.Entry `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Data) != 1 || body.Data[0].IntervalHrs != 6 || body.Data[0].Holders["13335"] != "Cloudflare" || body.Data[0].Path == "" {
		t.Fatalf("list = %+v", body.Data)
	}
}

// TestRefreshAsnSetFetchFailureIs502 covers the brief's "502 with the entry's
// error on failure": RIPEstat going away after a set exists.
func TestRefreshAsnSetFetchFailureIs502(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	cm, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	fail := false
	ripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":{"holder":"Cloudflare","prefixes":[{"prefix":"1.1.1.0/24"}]}}`))
	}))
	t.Cleanup(ripe.Close)
	f := asnsets.NewFetcher()
	f.BaseURL, f.RetryDelay = ripe.URL, 0
	h := &Handler{config: cm}
	h.SetASN(asnsets.NewManager(asnsets.NewStore(filepath.Join(dir, "asn.toml")), filepath.Join(dir, "asn"), f))
	r := chi.NewRouter()
	r.Post("/api/route/rule-sets/asn", h.CreateAsnSet)
	r.Post("/api/route/rule-sets/asn/{tag}/refresh", h.RefreshAsnSet)
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}}); rec.Code != http.StatusOK {
		t.Fatalf("create %d: %s", rec.Code, rec.Body)
	}
	fail = true
	rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn/cf/refresh", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("refresh with RIPEstat down: %d, want 502 (%s)", rec.Code, rec.Body)
	}
	e, ok := h.asn.Get("cf")
	if !ok || e.LastError == "" {
		t.Fatalf("last_error not recorded: %+v", e)
	}
	if _, err := os.Stat(filepath.Join(dir, "asn", "cf.json")); err != nil {
		t.Fatalf("old file must stay in service: %v", err)
	}
}

// TestAsnSetsUnwired: a handler without SetASN lists [] and refuses writes.
func TestAsnSetsUnwired(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.Get("/api/route/rule-sets/asn", h.ListAsnSets)
	r.Post("/api/route/rule-sets/asn", h.CreateAsnSet)
	rec := doJSON(t, r, "GET", "/api/route/rule-sets/asn", nil)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"data":[]`)) {
		t.Fatalf("unwired list: %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired create: %d", rec.Code)
	}
}

// TestWriteASNErrorMapping pins the status table: only fetch-side errors are
// 502; a store/file write failure is the caller's fallback (500), read-only 409.
func TestWriteASNErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{fmt.Errorf("%w: bad", asnsets.ErrInvalid), http.StatusBadRequest},
		{fmt.Errorf("x: %w", asnsets.ErrNotFound), http.StatusNotFound},
		{&asnsets.FetchError{ASN: 1, Err: errors.New("down")}, http.StatusBadGateway},
		{asnsets.ErrNoPrefixes, http.StatusBadGateway},
		{util.ReadOnlyError("/x/asn.toml"), http.StatusConflict},
		{errors.New("disk on fire"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeASNError(rec, http.StatusInternalServerError, c.err)
		if rec.Code != c.code {
			t.Errorf("%v: status %d, want %d", c.err, rec.Code, c.code)
		}
	}
}

// TestRefreshAsnSetEmptyUnionIs502: RIPEstat answering ok with zero prefixes
// for every AS is a fetch-side failure (502), and the old file stays.
func TestRefreshAsnSetEmptyUnionIs502(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	cm, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	empty := false
	ripe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if empty {
			_, _ = w.Write([]byte(`{"status":"ok","data":{"holder":"","prefixes":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":{"holder":"Cloudflare","prefixes":[{"prefix":"1.1.1.0/24"}]}}`))
	}))
	t.Cleanup(ripe.Close)
	f := asnsets.NewFetcher()
	f.BaseURL, f.RetryDelay = ripe.URL, 0
	h := &Handler{config: cm}
	h.SetASN(asnsets.NewManager(asnsets.NewStore(filepath.Join(dir, "asn.toml")), filepath.Join(dir, "asn"), f))
	r := chi.NewRouter()
	r.Post("/api/route/rule-sets/asn", h.CreateAsnSet)
	r.Post("/api/route/rule-sets/asn/{tag}/refresh", h.RefreshAsnSet)
	if rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn", map[string]any{"tag": "cf", "asns": []string{"13335"}}); rec.Code != http.StatusOK {
		t.Fatalf("create %d: %s", rec.Code, rec.Body)
	}
	empty = true
	rec := doJSON(t, r, "POST", "/api/route/rule-sets/asn/cf/refresh", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("refresh with empty union: %d, want 502 (%s)", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(filepath.Join(dir, "asn", "cf.json"))
	if err != nil || !bytes.Contains(data, []byte("1.1.1.0/24")) {
		t.Fatalf("old prefixes must stay in service: %v %s", err, data)
	}
}

// Minor 4: ASN request bodies are tiny (a tag and a few AS numbers); anything
// past 64 KiB is refused with 400 instead of being read into memory.
func TestAsnSetBodyTooLarge(t *testing.T) {
	h, r, _ := newASNHandler(t)
	big, _ := json.Marshal(map[string]any{"tag": "cf", "asns": []string{"13335"}, "pad": bytes.Repeat([]byte("x"), 70<<10)})
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/route/rule-sets/asn"},
		{"PUT", "/api/route/rule-sets/asn/cf"},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, bytes.NewReader(big)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s with a %d-byte body: status %d, want 400", c.method, c.path, len(big), rec.Code)
		}
	}
	if n := len(h.asn.List()); n != 0 {
		t.Fatalf("oversized create left %d sets", n)
	}
}

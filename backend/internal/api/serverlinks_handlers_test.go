package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"routebox/backend/internal/config"
)

func newLinkHandler(t *testing.T, seed string) *Handler {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}
	cfgMgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(cfgMgr, nil, "", nil, nil, nil, nil)
}

func linkRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/inbounds/{tag}/users/{userKey}/link", h.GetUserLink)
	return r
}

const linkSeedConfig = `{
  "inbounds": [
    {
      "type": "vless", "tag": "vless-in", "listen_port": 443,
      "tls": {
        "enabled": true, "server_name": "www.microsoft.com",
        "reality": {"enabled": true, "short_id": "0123abcd",
          "private_key": "SN5HcFLrdjYEYbYYowow0k8zRF5m2uvX6_vcun25p2s"}
      },
      "users": [{"name": "phone", "uuid": "11111111-2222-3333-4444-555555555555"}]
    }
  ]
}`

func TestGetUserLink(t *testing.T) {
	h := newLinkHandler(t, linkSeedConfig)
	srv := httptest.NewServer(linkRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/inbounds/vless-in/users/0/link?host=vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	data := decodeDataBytes(t, body)
	link, _ := data["link"].(string)
	if !strings.HasPrefix(link, "vless://11111111-2222-3333-4444-555555555555@vpn.example.com:443?") {
		t.Fatalf("unexpected link: %s", link)
	}
}

func TestGetUserLinkMissingHost(t *testing.T) {
	h := newLinkHandler(t, linkSeedConfig)
	srv := httptest.NewServer(linkRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/inbounds/vless-in/users/0/link")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Router mode: Server.PublicHost is a VPS/panel concept and legitimately empty on
// a router. The AWG client-config endpoints must resolve the client-facing host
// as Awg.ServerHost first, falling back to Server.PublicHost, and only 503 when
// BOTH are empty. Regression for "Failed to fetch client config: public host not
// configured" on routers.

// TestAWGPeerConfigUsesAwgServerHost: PublicHost empty + awg.server_host set ->
// the kernel .conf renders 200 with the AWG server host in the Endpoint.
func TestAWGPeerConfigUsesAwgServerHost(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{
		"server.public_host": "",
		"awg.server_host":    "192.168.1.200",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("conf with awg.server_host = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "192.168.1.200") {
		t.Fatalf("conf missing awg server host:\n%s", rec.Body.String())
	}
}

// TestAWGPeerSingboxUsesAwgServerHost: same fallback for the singbox JSON export.
func TestAWGPeerSingboxUsesAwgServerHost(t *testing.T) {
	h, r := newSingboxAWGHandler(t)
	if err := h.settings.Update(map[string]interface{}{
		"server.public_host": "",
		"awg.server_host":    "192.168.1.200",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/singbox", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("singbox export with awg.server_host = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "192.168.1.200") {
		t.Fatalf("export missing awg server host:\n%s", rec.Body.String())
	}
}

// TestAWGHostResolutionBothEmptyIs503: with neither awg.server_host nor
// server.public_host set, both endpoints still 503 (actionable message).
func TestAWGHostResolutionBothEmptyIs503(t *testing.T) {
	t.Run("conf", func(t *testing.T) {
		h, r := newAWGTestHandler(t)
		if err := h.settings.Update(map[string]interface{}{"server.public_host": ""}); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("both hosts empty = %d; want 503; body=%q", rec.Code, rec.Body.String())
		}
	})
	t.Run("singbox", func(t *testing.T) {
		h, r := newSingboxAWGHandler(t)
		if err := h.settings.Update(map[string]interface{}{"server.public_host": ""}); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/singbox", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("both hosts empty = %d; want 503; body=%q", rec.Code, rec.Body.String())
		}
	})
	t.Run("vpn-link", func(t *testing.T) {
		h, r := newAWGTestHandler(t)
		if err := h.settings.Update(map[string]interface{}{"server.public_host": ""}); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/vpn-link", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("both hosts empty = %d; want 503; body=%q", rec.Code, rec.Body.String())
		}
	})
}

// TestAWGHostFallsBackToPublicHost: awg.server_host empty + public_host set ->
// still 200 (VPS behavior unchanged).
func TestAWGHostFallsBackToPublicHost(t *testing.T) {
	_, r := newAWGTestHandler(t) // harness sets server.public_host=vpn.example.com
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback to public_host = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "vpn.example.com") {
		t.Fatalf("conf missing public_host fallback:\n%s", rec.Body.String())
	}
}

// awg.server_host takes PRECEDENCE over public_host when both are set — the AWG
// server address is a distinct concept from the panel's public host.
func TestAWGServerHostWinsOverPublicHost(t *testing.T) {
	h, r := newAWGTestHandler(t) // public_host=vpn.example.com
	if err := h.settings.Update(map[string]interface{}{"awg.server_host": "10.0.0.5"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("conf = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Endpoint = 10.0.0.5:51820") {
		t.Fatalf("awg.server_host must win over public_host:\n%s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/vpn-link", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("vpn-link: got %d, want 200", rec.Code)
	}
	// The resolved host must reach the payload — a status code cannot catch
	// "used server.public_host when awg.server_host was set".
	outer := decodeVPNLink(t, rec.Body.String())
	if outer["hostName"] != "10.0.0.5" {
		t.Fatalf("hostName = %v, want the awg.server_host value", outer["hostName"])
	}
	// last_config is the copy the client actually reads for its own state; the
	// outer hostName alone does not pin it (spec Testing §8).
	containers, ok := outer["containers"].([]any)
	if !ok || len(containers) != 1 {
		t.Fatalf("containers = %#v", outer["containers"])
	}
	cont, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatalf("container entry = %#v", containers[0])
	}
	// The fixture peer has no obfuscation configured, so this is plain wireguard.
	inner, ok := cont["wireguard"].(map[string]any)
	if !ok {
		t.Fatalf("container has no wireguard object: %#v", cont)
	}
	lastStr, ok := inner["last_config"].(string)
	if !ok {
		t.Fatalf("last_config must be a JSON string, got %T", inner["last_config"])
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(lastStr), &last); err != nil {
		t.Fatalf("last_config is not JSON: %v", err)
	}
	if last["hostName"] != "10.0.0.5" {
		t.Fatalf("last_config.hostName = %v, want 10.0.0.5", last["hostName"])
	}
	confStr, _ := last["config"].(string)
	if !strings.Contains(confStr, "Endpoint = 10.0.0.5:51820") {
		t.Fatalf("embedded .conf missing the resolved Endpoint line:\n%s", confStr)
	}
}

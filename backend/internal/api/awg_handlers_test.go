package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/awg"
	"routebox/backend/internal/config"
	"routebox/backend/internal/settings"
)

// Real 32-byte std-base64 keys (decode to exactly 32 bytes, so ValidatePublicKey
// accepts them as path params). The plan's literal "GOODPUB==" does NOT decode to
// 32 bytes, so it would 400 at the path-validation gate — these are the canonical
// fixtures.
const (
	knownPub  = "Yluwfrt+6ChDx8TJcdmDuw63AdoQDqA18LMVPr5b4Ks="
	knownPriv = "v4lHbWpEt1SZkmHfwXDcsFHhdyOTXCcZP0TiYysL1Qs="
	knownPSK  = "Q2hlY2tUaGlzUFNLSXNUaGlydHlUd29CeXRlc0xuZz0="
	// serverPriv: any 32-byte std-base64 is a valid x25519 private key.
	serverPriv      = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEs="
	validButUnknown = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
)

// awgNoopRunner satisfies awg.Runner without touching the system: Status() and
// AddPeer's `awg set` calls return empty output / nil error so the interface looks
// down and peer adds succeed.
type awgNoopRunner struct{}

func (awgNoopRunner) Run(_ context.Context, _ string, _ ...string) (string, string, error) {
	return "", "", nil
}

// newAWGTestHandler wires a Handler with an awg.Manager (noop runner + a seeded
// peer "knownPub" whose secret is known) and a real settings.Manager. Returns the
// handler and a chi router mounting the /api/awg routes (no auth — the auth gate
// is exercised separately by TestAWGRoutesRequireAuth).
func newAWGTestHandler(t *testing.T) (*Handler, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Seed a minimal server .conf so subnet/used reads find a file.
	if err := os.WriteFile(filepath.Join(awgDir, "awg-rb0.conf"),
		[]byte("[Interface]\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := awg.NewManagerForTest(awgNoopRunner{}, awgDir, serverPriv, awg.Config{
		Iface:      "awg-rb0",
		Subnet:     "10.10.0.0/24",
		ServerIP:   "10.10.0.1",
		ListenPort: 51820,
		MTU:        1420,
		DNS:        []string{"1.1.1.1"},
	})
	if err := m.Store().Put(awg.Peer{
		PublicKey:    knownPub,
		PrivateKey:   knownPriv,
		PresharedKey: knownPSK,
		Address:      "10.10.0.2/32",
		Name:         "phone",
	}); err != nil {
		t.Fatal(err)
	}

	sm := newAWGSettings(t, dir, "vpn.example.com")
	h := &Handler{settings: sm}
	h.SetAWG(m)

	r := chi.NewRouter()
	r.Route("/api/awg", func(r chi.Router) {
		r.Get("/status", h.GetAWGStatus)
		r.Post("/enable", h.EnableAWG)
		r.Post("/disable", h.DisableAWG)
		r.Get("/peers", h.ListAWGPeers)
		r.Post("/peers", h.CreateAWGPeer)
		// Mirrors main.go: the static segment must win over {publicKey}.
		r.Get("/peers/traffic", h.GetAWGPeersTraffic)
		r.Delete("/peers/{publicKey}", h.DeleteAWGPeer)
		r.Get("/peers/{publicKey}/config", h.GetAWGPeerConfig)
		r.Patch("/peers/{publicKey}/expiry", h.SetAWGPeerExpiry)
	})
	return h, r
}

func newAWGSettings(t *testing.T, dir, publicHost string) *settings.Manager {
	t.Helper()
	settingsPath := filepath.Join(dir, "routebox.toml")
	if err := os.WriteFile(settingsPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Load(); err != nil {
		t.Fatal(err)
	}
	if publicHost != "" {
		if err := sm.Update(map[string]interface{}{"server.public_host": publicHost}); err != nil {
			t.Fatal(err)
		}
	}
	return sm
}

func TestAWGConfigEndpointStatusCodes(t *testing.T) {
	h, r := newAWGTestHandler(t)

	// 1) {publicKey} not base64 -> 400 before any lookup (no traversal).
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/..%2Fetc%2Fpasswd/config", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-base64 pubkey = %d; want 400", rec.Code)
	}

	// 2) valid-shape but unknown pubkey -> 404 (existence before host).
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+validButUnknown+"/config", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown pubkey = %d; want 404", rec.Code)
	}

	// 3) known peer but public_host unset -> 503 (existence already passed).
	if err := h.settings.Update(map[string]interface{}{"server.public_host": ""}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no public_host = %d; want 503", rec.Code)
	}

	// 4) known peer + host set -> 200 text/plain, no-store, sanitized filename.
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing no-store")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="phone.conf"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if !strings.Contains(rec.Body.String(), "[Interface]") {
		t.Fatalf("body not a .conf:\n%s", rec.Body.String())
	}
}

func TestSetAWGPeerExpiryPastIs400(t *testing.T) {
	_, r := newAWGTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/awg/peers/"+knownPub+"/expiry",
		strings.NewReader(`{"expires_at":1}`)) // far in the past
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("past expiry = %d; want 400; body=%q", rec.Code, rec.Body.String())
	}
}

func TestSetAWGPeerExpiryUnknownIs404(t *testing.T) {
	_, r := newAWGTestHandler(t)
	future := time.Now().Add(48 * time.Hour).Unix()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/awg/peers/"+validButUnknown+"/expiry",
		strings.NewReader(fmt.Sprintf(`{"expires_at":%d}`, future)))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown peer = %d; want 404; body=%q", rec.Code, rec.Body.String())
	}
}

func TestSetAWGPeerExpirySetsIt(t *testing.T) {
	h, r := newAWGTestHandler(t)
	future := time.Now().Add(48 * time.Hour).Unix()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/awg/peers/"+knownPub+"/expiry",
		strings.NewReader(fmt.Sprintf(`{"expires_at":%d}`, future)))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set expiry = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	got, ok := h.awg.Store().Get(knownPub)
	if !ok || got.ExpiresAt != future {
		t.Fatalf("ExpiresAt not stored: %#v ok=%v", got, ok)
	}
}

// IPv6 public_host must be bracketed in the client Endpoint (BuildClient passes it
// verbatim). A bare-v6 "host:port" would be ambiguous/invalid.
func TestAWGConfigBracketsIPv6Endpoint(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "2001:db8::1"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Endpoint = [2001:db8::1]:51820") {
		t.Fatalf("IPv6 endpoint not bracketed:\n%s", rec.Body.String())
	}
}

// No private_key/preshared_key of any OTHER peer appears in responses, and the
// /peers summary list carries NO secret at all (mirrors /sub's no-secrets test).
func TestAWGNoSecretsInResponses(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}

	// The summary list must contain neither the private nor the preshared key.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers", nil))
	if body := rec.Body.String(); strings.Contains(body, knownPriv) || strings.Contains(body, knownPSK) {
		t.Fatalf("/api/awg/peers leaked a secret:\n%s", body)
	}

	// The create response is also a secret-free summary.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(`{"name":"laptop"}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "private_key") || strings.Contains(body, "preshared_key") {
		t.Fatalf("create response leaked a secret field:\n%s", body)
	}

	// The .conf legitimately contains the CLIENT's own priv/psk; assert it does NOT
	// contain the SERVER's private key (only the derived server pubkey).
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if strings.Contains(rec.Body.String(), serverPriv) {
		t.Fatalf("client .conf leaked the SERVER private key:\n%s", rec.Body.String())
	}
}

// POSTing peers into a /30 subnet exhausts it after one add (server holds .1,
// broadcast is .3, leaving only .2 usable). usedFromConf reads the on-disk .conf,
// which AddPeer appends to on success, so the second POST hits ErrSubnetExhausted
// -> the handler must map it to 409 Conflict.
func TestAWGCreatePeerSubnetExhausted(t *testing.T) {
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(awgDir, "awg-rb0.conf"),
		[]byte("[Interface]\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := awg.NewManagerForTest(awgNoopRunner{}, awgDir, serverPriv, awg.Config{
		Iface:      "awg-rb0",
		Subnet:     "10.10.0.0/30",
		ServerIP:   "10.10.0.1",
		ListenPort: 51820,
		MTU:        1420,
		DNS:        []string{"1.1.1.1"},
	})

	sm := newAWGSettings(t, dir, "vpn.example.com")
	h := &Handler{settings: sm}
	h.SetAWG(m)

	r := chi.NewRouter()
	r.Route("/api/awg", func(r chi.Router) {
		r.Post("/peers", h.CreateAWGPeer)
	})

	// First add fills the only usable host (.2).
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(`{"name":"first"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("first create = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}

	// Second add has no host left -> ErrSubnetExhausted -> 409 Conflict.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(`{"name":"second"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("exhausting create = %d; want 409; body=%q", rec.Code, rec.Body.String())
	}
}

// Every awg route requires auth (mirror TestTokenMutationRoutesRequireAuth):
// they are registered INSIDE the AuthMiddleware group, so WITHOUT panel
// credentials they must 401 before the handler runs. The read routes belong here
// too — they serve peer names, tunnel IPs, handshake times and per-minute
// activity patterns, which is not public information either.
func TestAWGRoutesRequireAuth(t *testing.T) {
	dir := t.TempDir()
	sm := newAWGSettings(t, dir, "vpn.example.com")
	const (
		username = "admin"
		password = "correct-horse-battery-staple"
	)
	if err := sm.Update(map[string]interface{}{
		"security.auth_enabled":  true,
		"security.auth_username": username,
		"security.auth_password": password,
	}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{settings: sm}
	sessions := auth.NewSessionStore()
	limiter := auth.NewLimiter()
	verifier := auth.NewCachedVerifier()
	h.SetAuth(sessions, limiter, verifier)

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(sm, sessions, limiter, verifier))
			r.Route("/awg", func(r chi.Router) {
				r.Post("/enable", h.EnableAWG)
				r.Post("/disable", h.DisableAWG)
				r.Get("/peers", h.ListAWGPeers)
				r.Post("/peers", h.CreateAWGPeer)
				r.Get("/peers/traffic", h.GetAWGPeersTraffic)
				r.Delete("/peers/{publicKey}", h.DeleteAWGPeer)
			})
		})
	})

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/awg/enable"},
		{http.MethodPost, "/api/awg/disable"},
		{http.MethodGet, "/api/awg/peers"},
		{http.MethodPost, "/api/awg/peers"},
		{http.MethodGet, "/api/awg/peers/traffic"},
		{http.MethodDelete, "/api/awg/peers/" + knownPub},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "203.0.113.9:1234"
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s = %d; want 401 (must be inside the auth group)", tc.method, tc.path, rec.Code)
		}
	}
}

// apiFakeSyncer satisfies awg.ConfigSyncer without a real config.Manager.
type apiFakeSyncer struct{ lastSpec *config.AwgServerSpec }

func (f *apiFakeSyncer) SyncAwgEndpointActive(_ string, spec *config.AwgServerSpec) (bool, error) {
	f.lastSpec = spec
	return true, nil
}

// newAWGSingboxHandler wires a Handler whose Manager runs the singbox backend
// (fake config syncer, capability gate open) so Enable/Disable are exercisable
// without kernel calls.
func newAWGSingboxHandler(t *testing.T) (*Handler, *apiFakeSyncer) {
	t.Helper()
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := awg.NewManagerForTest(awgNoopRunner{}, awgDir, serverPriv, awg.Config{
		Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1",
		ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
	})
	fs := &apiFakeSyncer{}
	m.SetBackend("singbox")
	m.SetConfigSync(fs, func() error { return nil }, func() bool { return true })
	sm := newAWGSettings(t, dir, "vpn.example.com")
	h := &Handler{settings: sm}
	h.SetAWG(m)
	return h, fs
}

// Bug C1: Enable/Disable must PERSIST awg.enabled so a RouteBox restart
// rehydrates the server in the state the operator left it (Disable must stick).
func TestAWGEnableDisablePersistEnabledFlag(t *testing.T) {
	h, _ := newAWGSingboxHandler(t)

	rec := httptest.NewRecorder()
	h.EnableAWG(rec, httptest.NewRequest(http.MethodPost, "/api/awg/enable", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !h.settings.Get().Awg.Enabled {
		t.Fatal("enable must persist awg.enabled=true")
	}

	rec = httptest.NewRecorder()
	h.DisableAWG(rec, httptest.NewRequest(http.MethodPost, "/api/awg/disable", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if h.settings.Get().Awg.Enabled {
		t.Fatal("disable must persist awg.enabled=false")
	}
}

// awgBlockingIPRunner blocks the Enable orchestrator at the WAN-detect step
// (`ip route show default`) until release is closed, holding it in flight. Every
// other command (awg show / iptables in Status) returns immediately.
type awgBlockingIPRunner struct{ release chan struct{} }

func (b awgBlockingIPRunner) Run(ctx context.Context, name string, _ ...string) (string, string, error) {
	if name == "ip" {
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return "", "", nil
}

// Bug I1: the awg.backend switch must be rejected while an Enable orchestrator is
// in flight — Status().Enabled stays false through every phase (a module install
// can take minutes), so the guard must also consult Busy().
func TestBackendSwitchRejectedWhileEnableInFlight(t *testing.T) {
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	m := awg.NewManagerForTest(awgBlockingIPRunner{release: release}, awgDir, serverPriv, awg.Config{
		Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1",
		ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
	})
	sm := newAWGSettings(t, dir, "")
	h := &Handler{settings: sm}
	h.SetAWG(m)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Kernel-branch enable: validation passes on these canonical values, then
		// WAN detect blocks on the runner until release is closed.
		_ = m.Enable(context.Background(), awg.EnableInput{
			Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
		})
	}()
	defer func() { close(release); <-done }()

	deadline := time.Now().Add(5 * time.Second)
	for !m.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("enable orchestrator never became busy")
		}
		time.Sleep(2 * time.Millisecond)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"awg.backend":"singbox"}`))
	h.UpdateSettings(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("backend switch mid-enable = %d; want 409; body=%q", rec.Code, rec.Body.String())
	}
	if got := sm.Get().Awg.Backend; got == "singbox" {
		t.Fatal("rejected switch must not persist awg.backend")
	}
}
// awgRecordingRunner records every argv (thread-safe not needed: handler path is
// synchronous) and answers everything with ("", nil).
type awgRecordingRunner struct{ calls [][]string }

func (r *awgRecordingRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return "", "", nil
}

func (r *awgRecordingRunner) saw(sub string) bool {
	for _, c := range r.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

// A validated kernel->singbox switch must (1) decommission the kernel runtime —
// at minimum clear awg-quick@'s boot persistence, which the Enabled==false guard
// alone never did (the reported bug: kernel kept running after the switch),
// (2) persist the new backend, (3) reset a stale-true awg.enabled so a restart
// can't rehydrate the NEW backend as silently enabled, and (4) mirror the switch
// onto the live Manager.
func TestBackendSwitchDecommissionsKernel(t *testing.T) {
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	run := &awgRecordingRunner{}
	m := awg.NewManagerForTest(run, awgDir, serverPriv, awg.Config{
		Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1",
		ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
	})
	m.SetBackend("kernel")
	sm := newAWGSettings(t, dir, "")
	// Stale persisted state: backend kernel, enabled=true, but the live probe sees
	// no iface (e.g. `awg` tool broken after a restart) — the guard passes.
	if err := sm.Update(map[string]interface{}{"awg.backend": "kernel", "awg.enabled": true}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{settings: sm}
	h.SetAWG(m)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"awg.backend":"singbox"}`))
	h.UpdateSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("switch = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !run.saw("systemctl disable --now awg-quick@awg-rb0") {
		t.Fatalf("switch must decommission the kernel unit; calls=%v", run.calls)
	}
	got := sm.Get().Awg
	if got.Backend != "singbox" {
		t.Fatalf("backend not persisted: %q", got.Backend)
	}
	if got.Enabled {
		t.Fatal("switch must reset the stale awg.enabled flag")
	}
	if m.BackendName() != "singbox" {
		t.Fatalf("live Manager backend = %q; want singbox", m.BackendName())
	}
}

// orchestrator input — the panel's Save is the single writer; Enable ignores body.
func TestAwgEnableInputFromSettings(t *testing.T) {
	in := awgEnableInput(settings.AwgSettings{
		Subnet: "10.20.0.0/24", ListenPort: 4500, MTU: 1380,
		DNS: []string{"9.9.9.9"}, WANIface: "ens3", ObfPreset: "standard",
		Obf: settings.AwgObf{Jc: 4, Jmin: 40, Jmax: 70, H1: "111", H2: "222", H3: "333", H4: "444",
			RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "25", MaxHandshakeAttempts: "18"},
	})
	if in.ListenPort != 4500 || in.Subnet != "10.20.0.0/24" || in.MTU != 1380 || in.WANIface != "ens3" {
		t.Fatalf("scalar fields not mapped: %+v", in)
	}
	if len(in.DNS) != 1 || in.DNS[0] != "9.9.9.9" {
		t.Fatalf("dns not mapped: %v", in.DNS)
	}
	if in.Obf.Jc != 4 || in.Obf.Jmax != 70 || in.Obf.H1 != "111" || in.Obf.H4 != "444" {
		t.Fatalf("obf not mapped: %+v", in.Obf)
	}
	if in.Obf.RekeyTimeout != "5" || in.Obf.RejectAfterTime != "180" ||
		in.Obf.KeepaliveTimeout != "25" || in.Obf.MaxHandshakeAttempts != "18" {
		t.Fatalf("device timers not mapped: %+v", in.Obf)
	}
}

func TestAwgEnableInputMapsIPv6Broker(t *testing.T) {
	in := awgEnableInput(settings.AwgSettings{IPv6Broker: true})
	if !in.IPv6Broker {
		t.Fatal("IPv6Broker not mapped from settings")
	}
}

// The wg-quick .conf now carries the awg3 fields (HeaderProtectionKey/CPA/RAT),
// so a singbox backend serves it regardless of the header-protection toggle —
// 200 in both states, no 409 gate. Kernel is unaffected (see next test).
func TestAWGPeerConfServedOnSingboxHeaderProtection(t *testing.T) {
	h, _ := newAWGSingboxHandler(t)
	if err := h.awg.Store().Put(awg.Peer{
		PublicKey:    knownPub,
		PrivateKey:   knownPriv,
		PresharedKey: knownPSK,
		Address:      "10.10.0.2/32",
		Name:         "phone",
	}); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Get("/api/awg/peers/{publicKey}/config", h.GetAWGPeerConfig)

	// singbox + header protection ON -> the awg3-capable .conf is valid -> 200.
	if err := h.settings.Update(map[string]interface{}{"awg.header_protection": true}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("singbox+HPK .conf = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}

	// singbox + header protection OFF -> 200 as before.
	if err := h.settings.Update(map[string]interface{}{"awg.header_protection": false}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("singbox no-HPK .conf = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
}

// The kernel backend never uses the header-protection key, so even a stale
// awg.header_protection=true in settings must NOT block the kernel .conf.
func TestAWGPeerConfKernelUnaffectedByHeaderProtection(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{"awg.header_protection": true}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+knownPub+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("kernel .conf with header_protection set = %d; want 200; body=%q", rec.Code, rec.Body.String())
	}
}

// The panel sends pubkeys URL-encoded (encodeURIComponent → %2B/%2F/%3D). The
// handler must URL-decode the path param before base64-validating; otherwise every
// key containing +,/,= 400s. Regression for the QR/.conf "HTTP 400" bug.
func TestAWGConfigDecodesEncodedPubkey(t *testing.T) {
	h, r := newAWGTestHandler(t)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}
	// knownPub = "Yluwfrt+6ChDx8TJcdmDuw63AdoQDqA18LMVPr5b4Ks=" encoded browser-style.
	encoded := "Yluwfrt%2B6ChDx8TJcdmDuw63AdoQDqA18LMVPr5b4Ks%3D"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/"+encoded+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("encoded pubkey config = %d; want 200 (handler must URL-decode); body=%q", rec.Code, rec.Body.String())
	}
}

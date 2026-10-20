package mtproto

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/9seconds/mtg/v2/mtglib"
	"github.com/9seconds/mtg/v2/mtglib/web"
)

// fakeFront records what the manager publishes, standing in for webfront.Front.
type fakeFront struct {
	mu    sync.Mutex
	last  http.Handler
	calls int
}

func (f *fakeFront) SetWebHandler(h http.Handler) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.last = h
	f.calls++
}

func (f *fakeFront) handler() http.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.last
}

func stub200(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("public"))
}

func webConfig(t *testing.T) Config {
	t.Helper()

	cfg := testConfig(t)
	cfg.Web = &WebConfig{Hostname: "proxy.example.com", Public: http.HandlerFunc(stub200)}

	return cfg
}

func webManager(t *testing.T) (*Manager, *fakeFront) {
	t.Helper()

	m := testManager(t)
	if err := m.Store().Put(Client{Name: "alice", Secret: secretA, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	front := &fakeFront{}
	m.SetWebFront(front)

	return m, front
}

func secretOf(keyHex, domain string) mtglib.Secret {
	var s mtglib.Secret

	for i := range s.Key {
		var b byte

		for _, c := range keyHex[i*2 : i*2+2] {
			b <<= 4

			switch {
			case c >= 'a':
				b |= byte(c-'a') + 10
			default:
				b |= byte(c - '0')
			}
		}

		s.Key[i] = b
	}

	s.Host = domain

	return s
}

// bridgeRequest crafts what the loopback front forwards: the Host the proxy is
// served on, the exact bridge query, a trusted peer and one forwarded client.
func bridgeRequest(keyHex string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/?bridge="+web.Capability(secretOf(keyHex, "example.com"), "proxy.example.com"), nil)
	r.Host = "proxy.example.com"
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")

	return r
}

func TestStartWithWebPublishesAHandlerToTheFront(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(webConfig(t)); err != nil {
		t.Fatal(err)
	}

	h := front.handler()
	if h == nil {
		t.Fatal("front got no handler")
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "proxy.example.com"
	r.RemoteAddr = "127.0.0.1:1"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK || rec.Body.String() != "public" {
		t.Errorf("got %d %q, want 200 from Public", rec.Code, rec.Body.String())
	}

	if !m.Status().Web.Enabled {
		t.Error("Status().Web.Enabled = false")
	}
}

func TestStartWithWebServesTheBridgeForAKnownClient(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(webConfig(t)); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	front.handler().ServeHTTP(rec, bridgeRequest(secretA))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); len(ct) < 9 || ct[:9] != "text/html" {
		t.Errorf("Content-Type = %q", ct)
	}

	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestStartWithWebRejectsABadHostname(t *testing.T) {
	m, _ := webManager(t)

	cfg := webConfig(t)
	cfg.Web.Hostname = "Proxy.Example.com."

	if err := m.Start(cfg); err == nil {
		t.Fatal("Start accepted a non-canonical hostname")
	}

	if m.Status().Running {
		t.Error("a failed Start left the proxy running")
	}

	// The listener must have been released again.
	l, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("port still held: %v", err)
	}

	_ = l.Close()
}

func TestStopRevertsTheFront(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(webConfig(t)); err != nil {
		t.Fatal(err)
	}

	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}

	if front.handler() != nil {
		t.Error("front still has the web handler after Stop")
	}

	if m.Status().Web.Enabled {
		t.Error("Web.Enabled after Stop")
	}
}

func TestRebuildToAnEmptyRosterRevertsTheFront(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(webConfig(t)); err != nil {
		t.Fatal(err)
	}

	if err := m.Store().Delete("alice"); err != nil {
		t.Fatal(err)
	}

	if err := m.Rebuild(); err == nil {
		t.Fatal("Rebuild with an empty roster succeeded")
	}

	if front.handler() != nil {
		t.Error("front still serves a revoked roster")
	}
}

func TestRebuildReindexesProfiles(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(webConfig(t)); err != nil {
		t.Fatal(err)
	}

	old := front.handler()

	if err := m.Store().Put(Client{Name: "bob", Secret: secretB, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := m.Rebuild(); err != nil {
		t.Fatal(err)
	}

	if front.handler() == nil || front.handler() == old {
		t.Fatal("Rebuild did not publish a fresh handler")
	}

	rec := httptest.NewRecorder()
	front.handler().ServeHTTP(rec, bridgeRequest(secretB))

	if rec.Code != http.StatusOK {
		t.Errorf("bridge for the new client = %d, want 200", rec.Code)
	}
}

func TestTokenKeyIsStablePerManager(t *testing.T) {
	m, _ := webManager(t)

	if len(m.tokenKey) != 32 {
		t.Fatalf("tokenKey len = %d", len(m.tokenKey))
	}

	first := string(m.tokenKey)

	cfg := webConfig(t)
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}

	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}

	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}

	if string(m.tokenKey) != first {
		t.Error("tokenKey changed across Starts")
	}

	if other := NewManager(NewStore("")); string(other.tokenKey) == first {
		t.Error("two managers share a tokenKey")
	}
}

func TestStatusWithoutWebIsZero(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	if got := m.Status().Web; got != (WebStatus{}) {
		t.Errorf("Web = %+v, want zero", got)
	}

	if front.handler() != nil {
		t.Error("front got a handler without Web config")
	}
}

func TestReconfigureAppliesTheNewConfig(t *testing.T) {
	m, front := webManager(t)

	if err := m.Start(testConfig(t)); err != nil {
		t.Fatal(err)
	}

	cfg := webConfig(t)
	if err := m.Reconfigure(cfg); err != nil {
		t.Fatal(err)
	}

	if front.handler() == nil || !m.Status().Web.Enabled {
		t.Error("Reconfigure did not enable the web proxy")
	}

	if got := m.Status().Listen; got != cfg.Listen {
		t.Errorf("Listen = %q, want the new %q", got, cfg.Listen)
	}

	c, err := net.Dial("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("new address not bound: %v", err)
	}

	_ = c.Close()
}

func TestReconfigureOnAStoppedManagerDoesNothing(t *testing.T) {
	m, _ := webManager(t)

	if err := m.Reconfigure(webConfig(t)); err != nil {
		t.Errorf("Reconfigure while stopped returned %v", err)
	}

	if m.Status().Running {
		t.Error("Reconfigure must not start a stopped proxy")
	}
}

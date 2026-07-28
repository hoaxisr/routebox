package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/awg"
)

// The kernel AWG backend keeps its server .conf next to peers.toml, in a system
// directory (/etc/amnezia/amneziawg) — the likeliest place for a read-only mount
// of the three. And it writes the .conf BEFORE the store, so the store's guard
// never gets a say: the peer operation failed on the .conf and reported "failed
// to add peer" with a 500, while the badge was already up because peers.toml had
// been probed. Same condition, two different answers, and the 500 named nothing.

// newReadOnlyKernelAWGHandler wires the kernel backend over a directory that is
// seeded first and locked afterwards, so reads still work and only writes fail.
func newReadOnlyKernelAWGHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	confPath := filepath.Join(awgDir, "awg-rb0.conf")
	if err := os.WriteFile(confPath, []byte("[Interface]\nListenPort = 51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(awgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(awgDir, 0o700) })

	m := awg.NewManagerForTest(awgNoopRunner{}, awgDir, serverPriv, awg.Config{
		Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1",
		ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
	})
	if m.BackendName() == "singbox" {
		t.Fatal("harness: this test is about the kernel backend")
	}

	h := &Handler{settings: newAWGSettings(t, dir, "vpn.example.com")}
	h.SetAWG(m)
	return h, confPath
}

func TestCreateAWGPeerKernelAnswers409WhenTheConfIsReadOnly(t *testing.T) {
	h, confPath := newReadOnlyKernelAWGHandler(t)

	rr := httptest.NewRecorder()
	h.CreateAWGPeer(rr, httptest.NewRequest(http.MethodPost, "/api/awg/peers",
		strings.NewReader(`{"name":"alice"}`)))

	assert409Naming(t, rr, confPath)
}

func TestDeleteAWGPeerKernelAnswers409WhenTheConfIsReadOnly(t *testing.T) {
	h, confPath := newReadOnlyKernelAWGHandler(t)

	r := chi.NewRouter()
	r.Delete("/api/awg/peers/{publicKey}", h.DeleteAWGPeer)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/awg/peers/"+knownPub, nil))

	assert409Naming(t, rr, confPath)
}

package webfront

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// freeAddr reserves a loopback port and hands it back, so a test can bind it.
func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer l.Close()

	return "127.0.0.1:" + strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

// textHandler answers every request with a fixed body.
func textHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	})
}

// get fetches addr's root over a fresh connection and returns the body.
func get(t *testing.T, addr string) (string, error) {
	t.Helper()

	c := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 3 * time.Second}

	resp, err := c.Get("http://" + addr + "/")
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)

	return string(b), err
}

func mustGet(t *testing.T, addr, want string) {
	t.Helper()

	got, err := get(t, addr)
	if err != nil || got != want {
		t.Fatalf("GET %s = %q, %v; want %q", addr, got, err, want)
	}
}

func startFront(t *testing.T, stub http.Handler) (*Front, string) {
	t.Helper()

	f := New(stub)
	t.Cleanup(func() { _ = f.Stop() })

	addr := freeAddr(t)
	if err := f.Start(addr); err != nil {
		t.Fatal(err)
	}

	return f, addr
}

func TestFrontServesTheStubUntilAHandlerIsSet(t *testing.T) {
	f, addr := startFront(t, textHandler("stub"))
	mustGet(t, addr, "stub")

	f.SetWebHandler(textHandler("web"))
	mustGet(t, addr, "web")
}

// The stub is what answers whenever the WEB proxy is off, so a new one has to
// take effect without waiting for a web handler to come and go.
func TestSetStubTakesEffectWithoutAWebHandler(t *testing.T) {
	f, addr := startFront(t, textHandler("old stub"))
	mustGet(t, addr, "old stub")
	f.SetStub(textHandler("new stub"))
	mustGet(t, addr, "new stub")
	// A web handler still wins over it, and reverting lands on the new stub.
	f.SetWebHandler(textHandler("web"))
	mustGet(t, addr, "web")
	f.SetWebHandler(nil)
	mustGet(t, addr, "new stub")
}

func TestSetWebHandlerSwapsAndNilReverts(t *testing.T) {
	f, addr := startFront(t, textHandler("stub"))

	f.SetWebHandler(textHandler("one"))
	mustGet(t, addr, "one")

	f.SetWebHandler(textHandler("two"))
	mustGet(t, addr, "two")

	f.SetWebHandler(nil)
	mustGet(t, addr, "stub")
}

func TestRebindMovesTheListener(t *testing.T) {
	f, old := startFront(t, textHandler("stub"))
	next := freeAddr(t)

	if err := f.Rebind(next); err != nil {
		t.Fatal(err)
	}

	if f.Listen() != next {
		t.Fatalf("Listen() = %q, want %q", f.Listen(), next)
	}

	mustGet(t, next, "stub")

	if _, err := get(t, old); err == nil {
		t.Fatal("old address still answers")
	}
}

func TestRebindToABusyPortKeepsTheOldListener(t *testing.T) {
	f, old := startFront(t, textHandler("stub"))

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer blocker.Close()

	if err := f.Rebind(blocker.Addr().String()); err == nil {
		t.Fatal("Rebind onto a busy port succeeded")
	}

	if f.Listen() != old {
		t.Fatalf("Listen() = %q, want %q", f.Listen(), old)
	}

	mustGet(t, old, "stub")
}

func TestStopReleasesThePort(t *testing.T) {
	f, addr := startFront(t, textHandler("stub"))

	if err := f.Stop(); err != nil {
		t.Fatal(err)
	}

	if f.Listen() != "" {
		t.Fatalf("Listen() = %q after Stop", f.Listen())
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port not released: %v", err)
	}

	_ = l.Close()

	if err := f.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStartTwiceIsRefused(t *testing.T) {
	f, _ := startFront(t, textHandler("stub"))

	if err := f.Start(freeAddr(t)); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("got %v, want ErrAlreadyRunning", err)
	}
}

func TestRebindWhenStoppedStarts(t *testing.T) {
	f := New(textHandler("stub"))
	t.Cleanup(func() { _ = f.Stop() })

	addr := freeAddr(t)
	if err := f.Rebind(addr); err != nil {
		t.Fatal(err)
	}

	mustGet(t, addr, "stub")
}

func TestRebindToTheSameAddressIsANoop(t *testing.T) {
	f, addr := startFront(t, textHandler("stub"))

	if err := f.Rebind(addr); err != nil {
		t.Fatal(err)
	}

	if f.Listen() != addr {
		t.Fatalf("Listen() = %q, want %q", f.Listen(), addr)
	}

	mustGet(t, addr, "stub")
}

func TestRebindDoesNotWaitForInFlightRequests(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})

	f, old := startFront(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "slow")
	}))

	type result struct {
		body string
		err  error
	}

	done := make(chan result, 1)

	go func() {
		body, err := get(t, old)
		done <- result{body, err}
	}()

	<-started

	rebound := make(chan error, 1)

	go func() { rebound <- f.Rebind(freeAddr(t)) }()

	select {
	case err := <-rebound:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Rebind blocked on an in-flight request")
	}

	close(release)

	if r := <-done; r.err != nil || r.body != "slow" {
		t.Fatalf("in-flight request got %q, %v", r.body, r.err)
	}
}

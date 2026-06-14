package api

import (
	"bufio"
	"bytes"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubTokenScrubber_RedactsToken(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	var seenPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path // handler must still see the REAL path
		w.WriteHeader(http.StatusOK)
	})

	mw := SubTokenScrubber(logger)(next)

	const secret = "SUPERSECRETTOKEN1234567890"
	req := httptest.NewRequest(http.MethodGet, "/sub/"+secret, nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("token leaked into log output: %q", out)
	}
	if !strings.Contains(out, "/sub/<redacted>") {
		t.Fatalf("log did not contain redacted sub path, got: %q", out)
	}
	if seenPath != "/sub/"+secret {
		t.Fatalf("handler saw scrubbed path %q, want real path", seenPath)
	}
}

func TestSubTokenScrubber_LeavesOtherPaths(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := SubTokenScrubber(logger)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), "/api/users") {
		t.Fatalf("non-sub path was not logged verbatim: %q", buf.String())
	}
}

// TestScrubSubPath exercises every branch of scrubSubPath, including the
// security-critical /subscriptions false-positive guard.
func TestScrubSubPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"token redacted", "/sub/SUPERSECRETTOKEN", "/sub/<redacted>"},
		{"bare sub unchanged", "/sub", "/sub"},
		{"trailing slash unchanged", "/sub/", "/sub/"},
		{"token with extra segment redacted", "/sub/SECRET/extra", "/sub/<redacted>"},
		{"subscriptions verbatim", "/subscriptions/foo", "/subscriptions/foo"},
		{"api path verbatim", "/api/users", "/api/users"},
		{"root verbatim", "/", "/"},
		{"uppercase prefix redacted", "/SUB/SECRET", "/sub/<redacted>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scrubSubPath(tt.in)
			if got != tt.want {
				t.Fatalf("scrubSubPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// For redacted cases the original token must never survive.
			if tt.want == "/sub/<redacted>" && strings.Contains(got, "SECRET") {
				t.Fatalf("token leaked through scrubSubPath: %q", got)
			}
		})
	}
}

// TestSubTokenScrubber_CapturesNonDefaultStatus ensures the recorder reports the
// real status written by the handler (not the http.StatusOK default).
func TestSubTokenScrubber_CapturesNonDefaultStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mw := SubTokenScrubber(logger)(next)

	req := httptest.NewRequest(http.MethodGet, "/sub/TOKEN", nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), "404") {
		t.Fatalf("log did not capture non-default status 404, got: %q", buf.String())
	}
}

// TestSubTokenScrubber_RedactsTokenWithQuery confirms the query string cannot
// carry the token into the log (r.URL.Path drops the query) and the path is
// still redacted.
func TestSubTokenScrubber_RedactsTokenWithQuery(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := SubTokenScrubber(logger)(next)

	const secret = "SUPERSECRETTOKEN1234567890"
	req := httptest.NewRequest(http.MethodGet, "/sub/"+secret+"?x=y", nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("token leaked into log output with query string: %q", out)
	}
	if !strings.Contains(out, "/sub/<redacted>") {
		t.Fatalf("log did not contain redacted sub path, got: %q", out)
	}
}

// hijackFlushRecorder is a fake ResponseWriter that implements both
// http.Hijacker and http.Flusher so we can assert statusRecorder promotes them.
type hijackFlushRecorder struct {
	http.ResponseWriter
	hijacked bool
	flushed  bool
}

func (h *hijackFlushRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, errors.New("fake hijack")
}

func (h *hijackFlushRecorder) Flush() { h.flushed = true }

// TestStatusRecorderForwardsHijacker verifies the recorder satisfies and
// delegates http.Hijacker and http.Flusher when the wrapped writer does — this
// is what keeps the Clash WS upgrades and SSE flushing working when the
// middleware sits at the router root.
func TestStatusRecorderForwardsHijacker(t *testing.T) {
	fake := &hijackFlushRecorder{ResponseWriter: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: fake, status: http.StatusOK}

	hj, ok := interface{}(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder does not satisfy http.Hijacker")
	}
	if _, _, err := hj.Hijack(); err == nil || err.Error() != "fake hijack" {
		t.Fatalf("Hijack did not delegate to underlying writer, err=%v", err)
	}
	if !fake.hijacked {
		t.Fatal("underlying Hijack was not called")
	}

	fl, ok := interface{}(rec).(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder does not satisfy http.Flusher")
	}
	fl.Flush()
	if !fake.flushed {
		t.Fatal("underlying Flush was not called")
	}
}

// TestStatusRecorderHijackNotSupported confirms a graceful error when the
// wrapped writer cannot be hijacked.
func TestStatusRecorderHijackNotSupported(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if _, _, err := rec.Hijack(); err != http.ErrNotSupported {
		t.Fatalf("expected http.ErrNotSupported, got %v", err)
	}
	// Flush on a non-flusher must be a no-op (no panic).
	rec.Flush()
}

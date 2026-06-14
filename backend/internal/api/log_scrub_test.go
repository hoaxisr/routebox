package api

import (
	"bytes"
	"log"
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

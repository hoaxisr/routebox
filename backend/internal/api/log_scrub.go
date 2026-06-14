package api

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// SubTokenScrubber is the ROOT access logger. It REPLACES chi's middleware.Logger
// so that /sub/<token> request paths are logged as /sub/<redacted> — the token is
// a credential and must NEVER reach the log. We compute the log line ourselves
// from a scrubbed path; chi's own logger formats its line from r.RequestURI, so a
// mere URL.Path rewrite would still leak the token — instead we own the logging
// and forward the UNMODIFIED request downstream, so chi's {token} URL param still
// resolves correctly. All non-/sub paths are logged verbatim.
func SubTokenScrubber(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r) // real request, real path — handler unaffected
			logger.Printf("%s %s %d %s",
				r.Method, scrubSubPath(r.URL.Path), ww.status, time.Since(start))
		})
	}
}

// scrubSubPath replaces the token segment of /sub/<token> with <redacted>. It
// matches ONLY the "/sub/" prefix (NOT "/subscriptions/..."), leaving /sub and
// /sub/ untouched and every other path verbatim.
func scrubSubPath(path string) string {
	if path == "/sub" || path == "/sub/" {
		return path
	}
	if strings.HasPrefix(path, "/sub/") {
		return "/sub/<redacted>"
	}
	return path
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

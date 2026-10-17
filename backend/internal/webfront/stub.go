// Package webfront is the plain-HTTP server on a loopback address that sits
// behind a TLS-terminating reverse proxy (Caddy on all-in-one installs, the
// operator's own proxy elsewhere). It serves RouteBox's stub site, or, once
// one is set, the Telegram WEB proxy handler of the mtg fork.
package webfront

import (
	"net/http"

	"github.com/9seconds/mtg/v2/mtglib/web"
)

// NewStub serves dir the way Caddy's file_server did for the stub site, using
// the same static handler mtg serves its public directory with. A missing dir
// is not an error: every request is then an empty 404, so the front still
// comes up on a host that has no stub site installed.
func NewStub(dir string) http.Handler {
	h, err := web.NewPublicDir(dir)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusNotFound)
		})
	}

	return h
}

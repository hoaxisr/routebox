package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/process"
)

// SpeedTestOutbound measures throughput and responsiveness through one outbound
// (#13), using the binary's own `tools networkquality` — Apple's RPM
// methodology — rather than a home-grown download loop.
//
// Synchronous on purpose. The run is bounded to well under half a minute, one
// runs at a time, and the alternative is a job registry plus a progress channel
// for a button pressed once in a while. The client sets its own longer timeout.
func (h *Handler) SpeedTestOutbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	if tag == "" {
		writeError(w, http.StatusBadRequest, "outbound tag is required")
		return
	}

	// The APPLIED config, not the draft: this measures the outbound as it is
	// actually dialled. An outbound that exists only in unsaved changes is not
	// reachable yet, and the binary says so by name.
	configPath := h.config.GetPath()
	if configPath == "" {
		writeError(w, http.StatusServiceUnavailable, "no config file to test against")
		return
	}

	result, err := h.process.RunSpeedTest(r.Context(), configPath, tag)
	if err != nil {
		if errors.Is(err, process.ErrSpeedTestBusy) {
			// 429, NOT 409: in this API 409 means a write was refused because the
			// config is read-only, and the client reacts to it by re-reading that
			// state. "Come back in a moment" is a different thing entirely.
			writeError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		// 502 is for a measurement that ran and failed out there. Being unable to
		// read our own config, or having no binary to run, is ours.
		if strings.HasPrefix(err.Error(), "read config:") ||
			strings.Contains(err.Error(), "no amnezia-box binary") ||
			strings.HasPrefix(err.Error(), "parse config:") {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeSuccess(w, result)
}

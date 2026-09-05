package api

import (
	"net/http"
)

// GetSystem serves host metrics for the dashboard strips: CPU, memory, load,
// disk and the managed process's RSS. Polled every couple of seconds while
// the page is open; CPU is a delta between polls, so the first answer after a
// RouteBox start carries cpu_percent: null.
func (h *Handler) GetSystem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeSuccess(w, h.sys.Snapshot(h.getProcessStatus().PID))
}

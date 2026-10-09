package api

import (
	"encoding/json"
	"net/http"
	"sync"
)

// applyProgress lets the panel follow an apply whose response it may never
// get: the reload drops every connection through sing-box, including a phone
// that reaches the panel through a VPN served by this same sing-box. The
// panel snapshots seq before POSTing and polls GET /config/apply/progress, so
// it shows the current phase live and learns the outcome from there.
type applyProgress struct {
	mu      sync.Mutex
	seq     int
	phase   string // "" (never ran since start), checking, reloading, done, error
	err     string
	warning string // the dest/naive warning a successful apply may carry
}

func (p *applyProgress) start() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	p.phase, p.err, p.warning = "checking", "", ""
	return p.seq
}

// set ignores a run that a newer apply has already replaced.
func (p *applyProgress) set(seq int, phase, errMsg, warning string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if seq == p.seq {
		p.phase, p.err, p.warning = phase, errMsg, warning
	}
}

func (p *applyProgress) snapshot() map[string]interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := map[string]interface{}{"seq": p.seq, "phase": p.phase}
	if p.err != "" {
		m["error"] = p.err
	}
	if p.warning != "" {
		m["warning"] = p.warning
	}
	return m
}

// applyRecorder catches what ApplyConfig answered, so the outcome lands in
// applyProgress no matter which of its many return paths produced it.
type applyRecorder struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (r *applyRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *applyRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

func (r *applyRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *applyRecorder) outcome() (phase, errMsg, warning string) {
	if r.status == 0 {
		// Nothing written: the handler panicked and Recoverer answered below us.
		return "error", "apply crashed, see the RouteBox log", ""
	}
	var resp struct {
		Error string `json:"error"`
		Data  struct {
			Warning string `json:"warning"`
		} `json:"data"`
	}
	parsed := json.Unmarshal(r.body, &resp) == nil
	if r.status == http.StatusOK {
		return "done", "", resp.Data.Warning
	}
	if parsed && resp.Error != "" {
		return "error", resp.Error, ""
	}
	return "error", http.StatusText(r.status), ""
}

// GetApplyProgress answers the panel's once-a-second poll during an apply.
// Deliberately not part of /config/status: that one diffs the draft, which
// forks diff(1) — too much to run every second on a small router.
func (h *Handler) GetApplyProgress(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, h.applyProg.snapshot())
}

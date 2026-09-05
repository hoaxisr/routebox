package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"routebox/backend/internal/process"
)

// The dashboard's system strip: host metrics plus the managed process's RSS,
// read for the pid the status reports (here: the test binary itself).
func TestGetSystemReportsHostAndProcess(t *testing.T) {
	h := &Handler{statusSource: func() process.Status {
		return process.Status{Running: true, PID: os.Getpid()}
	}}
	rec := httptest.NewRecorder()
	h.GetSystem(rec, httptest.NewRequest(http.MethodGet, "/api/system", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			MemTotal   uint64 `json:"mem_total"`
			MemUsed    uint64 `json:"mem_used"`
			Cores      int    `json:"cores"`
			ProcessRSS uint64 `json:"process_rss"`
			DiskTotal  uint64 `json:"disk_total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	d := resp.Data
	if d.MemTotal == 0 || d.MemUsed == 0 || d.Cores == 0 || d.ProcessRSS == 0 || d.DiskTotal == 0 {
		t.Fatalf("zero metric: %+v", d)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
}

// Process down => no pid => RSS 0, the rest still there.
func TestGetSystemWithoutProcess(t *testing.T) {
	h := &Handler{statusSource: func() process.Status { return process.Status{} }}
	rec := httptest.NewRecorder()
	h.GetSystem(rec, httptest.NewRequest(http.MethodGet, "/api/system", nil))
	var resp struct {
		Data struct {
			MemTotal   uint64 `json:"mem_total"`
			ProcessRSS uint64 `json:"process_rss"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Data.ProcessRSS != 0 || resp.Data.MemTotal == 0 {
		t.Fatalf("%+v", resp.Data)
	}
}

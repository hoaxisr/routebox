package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The confirm dialog (breakdown.resetConfirmBody) promises to delete *all*
// accumulated traffic statistics. That includes the per-user totals shown on
// the Users page and in the subscription userinfo header, not just the
// connection-level traffic_minute series.
func TestResetTrafficHistory_ClearsPerUserTotals(t *testing.T) {
	store := openAPITrafficStore(t)
	now := time.Now().Unix()
	if err := store.Upsert(now-300, "10.0.0.2", "example.com", "direct", 100, 200); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(now-300, "alice", 100, 200); err != nil {
		t.Fatal(err)
	}

	h := &Handler{traffic: store}
	rec := httptest.NewRecorder()
	h.ResetTrafficHistory(rec, httptest.NewRequest(http.MethodPost, "/api/traffic/reset", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset returned %d: %s", rec.Code, rec.Body.String())
	}

	rows, err := store.QueryAggregate(now-86400, now, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("connection history survived the reset: %+v", rows)
	}

	up, down, err := store.QueryUserTotals(now-86400, now, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if up != 0 || down != 0 {
		t.Errorf("per-user totals survived the reset: %d/%d, want 0/0", up, down)
	}
}

// A failure in either half must surface as an error naming the part that
// failed — a silent partial success is exactly the defect being fixed.
func TestResetTrafficHistory_ReportsFailingPart(t *testing.T) {
	store := openAPITrafficStore(t)
	// Closing the DB makes both DELETEs fail.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	h := &Handler{traffic: store}
	rec := httptest.NewRecorder()
	h.ResetTrafficHistory(rec, httptest.NewRequest(http.MethodPost, "/api/traffic/reset", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("reset returned %d, want 500: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "connection history") {
		t.Errorf("error does not name the connection-history part: %s", body)
	}
	if !strings.Contains(body, "per-user") {
		t.Errorf("error does not name the per-user part: %s", body)
	}
}

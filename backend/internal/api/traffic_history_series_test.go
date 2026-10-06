package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"routebox/backend/internal/traffic"
)

// ?series=1 adds the whole-network time series next to the breakdown buckets;
// without it the response shape is unchanged (#99).
func TestGetTrafficHistory_Series(t *testing.T) {
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	h := &Handler{traffic: store}

	now := time.Now().Unix()
	bucket := (now/60)*60 - 120
	for _, r := range []struct {
		src      string
		up, down int64
	}{{"10.0.0.2", 1, 2}, {"192.168.1.7", 10, 20}} {
		if err := store.Upsert(bucket, r.src, "a.example", "direct", r.up, r.down); err != nil {
			t.Fatal(err)
		}
	}

	get := func(q string) map[string]json.RawMessage {
		rec := httptest.NewRecorder()
		h.GetTrafficHistory(rec, httptest.NewRequest(http.MethodGet, "/api/traffic/history"+q, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", q, rec.Code, rec.Body)
		}
		var env struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env.Data
	}

	if _, has := get("?range=1h")["series"]; has {
		t.Fatal("series present without ?series=1")
	}
	data := get("?range=1h&series=1")
	var series []traffic.UserHistoryRow
	if err := json.Unmarshal(data["series"], &series); err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0] != (traffic.UserHistoryRow{BucketTs: bucket, Upload: 11, Download: 22}) {
		t.Fatalf("series = %+v", series)
	}
	if string(data["step"]) != "60" {
		t.Fatalf("step = %s, want 60", data["step"])
	}
	// The route graph's split rides along: download per final outbound (#110).
	var leaves []traffic.LeafHistoryRow
	if err := json.Unmarshal(data["leaves"], &leaves); err != nil {
		t.Fatal(err)
	}
	if len(leaves) != 1 || leaves[0] != (traffic.LeafHistoryRow{BucketTs: bucket, Leaf: "direct", Download: 22}) {
		t.Fatalf("leaves = %+v", leaves)
	}
	// The source filter narrows the series like it narrows the buckets.
	if err := json.Unmarshal(get("?range=1h&series=1&source=10.0.0.2")["series"], &series); err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Upload != 1 {
		t.Fatalf("filtered series = %+v", series)
	}
	// An idle window carries no series key (never null) — step still says
	// the request was understood.
	data = get("?range=1h&series=1&source=nobody")
	if got, has := data["series"]; has {
		t.Fatalf("empty series = %s, want absent", got)
	}
	if string(data["step"]) != "60" {
		t.Fatalf("step on idle window = %s", data["step"])
	}
}

package consumers

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"routebox/backend/internal/traffic"
	"routebox/backend/internal/users"
	"routebox/backend/internal/v2stats"
)

func openStore(t *testing.T) *traffic.Store {
	t.Helper()
	s, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

type fakeStats map[string]v2stats.Counters

func (f fakeStats) QueryUsersTimeout(time.Duration) (map[string]v2stats.Counters, error) {
	return f, nil
}

type failingStats struct{ err error }

func (f failingStats) QueryUsersTimeout(time.Duration) (map[string]v2stats.Counters, error) {
	return nil, f.err
}

func TestUserSourceSumsAllTrafficNames(t *testing.T) {
	store := openStore(t)
	_ = store.UpsertUser(60, "ivan", 10, 100)
	_ = store.UpsertUser(60, "ivan-hy2", 1, 1000)
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "u1", Name: "ivan", Enabled: true, Bindings: []users.Binding{
		{Name: "ivan", Protocol: "vless"}, {Name: "ivan-hy2", Protocol: "hysteria2"},
	}})
	src := &UserSource{Users: mgr, Store: store, Now: func() int64 { return 120 },
		Stats: fakeStats{"ivan": {Uplink: 1, Downlink: 2}, "ivan-hy2": {Uplink: 10, Downlink: 20}}}

	if src.Kind() != "user" {
		t.Fatalf("kind = %q", src.Kind())
	}
	rows, err := src.List(0, 120)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	r := rows[0]
	if r.Kind != "user" || r.ID != "u1" || r.Name != "ivan" || r.Upload != 11 || r.Download != 1100 || r.State != StateActive {
		t.Fatalf("row = %+v", r)
	}
	if len(r.Tags) != 2 || r.Tags[0] != "vless" || r.Tags[1] != "hysteria2" {
		t.Fatalf("tags = %v", r.Tags)
	}
	if len(r.History) == 0 {
		t.Fatalf("history must be populated from the store, row = %+v", r)
	}
	c, err := src.Counters(context.Background())
	if err != nil || c["u1"] != (Counter{Up: 11, Down: 22}) {
		t.Fatalf("counters = %v err=%v", c, err)
	}
}

func TestUserSourceStates(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "off", Name: "a", Enabled: false})
	_ = mgr.Put(&users.PanelUser{ID: "full", Name: "b", Enabled: true, QuotaBytes: 10, UsedRx: 10})
	_ = mgr.Put(&users.PanelUser{ID: "gone", Name: "c", Enabled: true, ExpiresAt: 1})
	rows, _ := (&UserSource{Users: mgr, Now: func() int64 { return 1 }}).List(0, 1)
	got := map[string]string{}
	for _, r := range rows {
		got[r.ID] = r.State + "/" + r.StateReason
	}
	if got["off"] != "disabled/" || got["full"] != "suspended/quota" || got["gone"] != "suspended/expired" {
		t.Fatalf("states = %v", got)
	}
	// Manager.List is sorted by name, so the page is stable across reloads.
	if rows[0].ID != "off" || rows[1].ID != "full" || rows[2].ID != "gone" {
		t.Fatalf("order = %v", []string{rows[0].ID, rows[1].ID, rows[2].ID})
	}
	// Quota fields travel with the row so the page can draw the bar.
	for _, r := range rows {
		if r.ID == "full" && (r.QuotaBytes != 10 || r.Used != 10) {
			t.Fatalf("quota fields = %+v", r)
		}
	}
}

func TestUserSourceDedupesProtocolTags(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "u", Name: "u", Enabled: true, Bindings: []users.Binding{
		{Name: "u", Protocol: "vless"}, {Name: "u-2", Protocol: "vless"}, {Name: "u-3", Protocol: ""},
	}})
	rows, err := (&UserSource{Users: mgr, Now: func() int64 { return 1 }}).List(0, 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if len(rows[0].Tags) != 1 || rows[0].Tags[0] != "vless" {
		t.Fatalf("tags = %v", rows[0].Tags)
	}
}

func TestUserSourceNilManagerIsNotAnError(t *testing.T) {
	src := &UserSource{}
	rows, err := src.List(0, 1)
	if err != nil || rows != nil {
		t.Fatalf("List = %v err=%v", rows, err)
	}
	c, err := src.Counters(context.Background())
	if err != nil || c != nil {
		t.Fatalf("Counters = %v err=%v", c, err)
	}
}

func TestUserSourceWithoutUsersIsNotAnError(t *testing.T) {
	c, err := (&UserSource{Users: users.NewManager("")}).Counters(context.Background())
	if err != nil || c != nil {
		t.Fatalf("got %v err=%v", c, err)
	}
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "u1", Name: "a", Enabled: true})
	if _, err := (&UserSource{Users: mgr}).Counters(context.Background()); err == nil {
		t.Fatal("users exist but no v2ray_api: that is unavailable, not empty")
	}
}

func TestUserSourceCountersErrorIsStable(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "u1", Name: "a", Enabled: true})

	// No client at all: the same text every tick, so the sampler logs once.
	src := &UserSource{Users: mgr}
	_, err1 := src.Counters(context.Background())
	_, err2 := src.Counters(context.Background())
	if err1 == nil || err2 == nil || err1.Error() != err2.Error() {
		t.Fatalf("err1=%v err2=%v", err1, err2)
	}

	// A failing query keeps the cause and does not invent a varying message.
	cause := errors.New("rpc error: code = Unavailable desc = connection refused")
	src = &UserSource{Users: mgr, Stats: failingStats{err: cause}}
	_, err1 = src.Counters(context.Background())
	_, err2 = src.Counters(context.Background())
	if err1 == nil || !errors.Is(err1, cause) || err1.Error() != err2.Error() {
		t.Fatalf("err1=%v err2=%v", err1, err2)
	}
}

func TestUserSourceCountersMissingNamesCountAsZero(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "u1", Name: "a", Enabled: true})
	_ = mgr.Put(&users.PanelUser{ID: "u2", Name: "b", Enabled: true})
	src := &UserSource{Users: mgr, Stats: fakeStats{"a": {Uplink: 3, Downlink: 4}}}
	c, err := src.Counters(context.Background())
	if err != nil || len(c) != 2 || c["u1"] != (Counter{Up: 3, Down: 4}) || c["u2"] != (Counter{}) {
		t.Fatalf("counters = %v err=%v", c, err)
	}
}

// Panel users only exist in vps mode: a disabled source has no rows and
// nothing to count, and it never touches the stats client.
func TestUserSourceDisabledHasNothing(t *testing.T) {
	mgr := users.NewManager("")
	_ = mgr.Put(&users.PanelUser{ID: "u1", Name: "ivan", Enabled: true, Bindings: []users.Binding{{Name: "ivan", Protocol: "vless"}}})
	src := &UserSource{Users: mgr, Enabled: func() bool { return false },
		Stats: failingStats{err: errors.New("must not be called")}}
	rows, err := src.List(0, 120)
	if rows != nil || err != nil {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	c, err := src.Counters(context.Background())
	if c != nil || err != nil {
		t.Fatalf("counters=%v err=%v", c, err)
	}
}

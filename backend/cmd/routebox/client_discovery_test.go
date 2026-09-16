package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"routebox/backend/internal/clients"
	"routebox/backend/internal/util"
)

// Issue #102: discovery fed every sourceIP Clash reports into the roster, so a
// connection from a public address became an entry on /config/clients — a
// device nobody owns and nobody can name. Only local sources are observed now;
// the IPv4-mapped spelling of a LAN address still counts as one (#71).
func TestRunClientDiscovery_OnlyLocalSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"connections":[
			{"metadata":{"sourceIP":"192.168.1.14"}},
			{"metadata":{"sourceIP":"::ffff:10.10.64.2"}},
			{"metadata":{"sourceIP":"172.217.116.4"}},
			{"metadata":{"sourceIP":""}}
		]}`))
	}))
	defer srv.Close()

	mgr := clients.New("") // memory only
	stop := make(chan struct{})
	defer close(stop)
	isLocal := func(ip string) bool { return util.IsLocalClientIP(ip) }
	go runClientDiscovery(mgr, strings.TrimPrefix(srv.URL, "http://"), "", isLocal, stop)

	// The loop polls once immediately, then every 60s — wait for that first pass
	// to observe BOTH local sources: stopping at the first entry could catch the
	// roster between the two Observe calls and fail for timing, not behaviour.
	deadline := time.Now().Add(5 * time.Second)
	for len(mgr.List()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	got := map[string]bool{}
	for _, e := range mgr.List() {
		got[e.IP] = true
	}
	for _, want := range []string{"192.168.1.14", "10.10.64.2"} {
		if !got[want] {
			t.Errorf("roster is missing local client %s: %v", want, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("roster = %v, want only the two local clients", got)
	}
}

// The sampler's filter follows the mode that is in force right now. --mode can
// set it while the settings file says nothing (the VPS image passes the flag on
// every boot, and a /config volume from an older image has no mode in its toml),
// and the panel can change the setting at runtime, where the field promises the
// change takes effect on save. Either way a box that is a panel must keep
// recording its remote clients (#102 review).
func TestSamplerKeepSource_ByMode(t *testing.T) {
	mode := "router"
	keep := samplerKeepSource(func() string { return mode }, func(ip string) bool {
		return ip == "192.168.1.14"
	})

	if !keep("192.168.1.14") || keep("172.217.116.4") {
		t.Error("router mode must record local sources and only those")
	}
	mode = "vps"
	if !keep("172.217.116.4") {
		t.Error("vps mode must record every source — its clients are remote")
	}
	mode = ""
	if keep("172.217.116.4") {
		t.Error("an unset mode is router, not vps")
	}
}

// The flag beats the settings file, and an absent mode is router. The VPS image
// passes --mode vps on every boot while a /config volume from an older image can
// have no [server] section at all: reading settings alone there would turn the
// router filter on and leave a panel recording nothing (#102 review).
func TestResolveMode(t *testing.T) {
	cases := []struct {
		flag, settings, want string
	}{
		{"vps", "", "vps"},          // the VPS image: flag only
		{"vps", "router", "vps"},    // stale toml from an older image
		{"", "vps", "vps"},          // generic binary, mode switched in the panel
		{"router", "vps", "router"}, // operator overrides the file on purpose
		{"", "", "router"},          // nothing said anywhere
		{"", "router", "router"},
	}
	for _, c := range cases {
		if got := resolveMode(c.flag, func() string { return c.settings }); got != c.want {
			t.Errorf("resolveMode(flag=%q, settings=%q) = %q, want %q", c.flag, c.settings, got, c.want)
		}
	}
}

// And the settings side is read per call, not captured once: the field in the
// panel says the change takes effect on save.
func TestResolveMode_ReadsSettingsEachCall(t *testing.T) {
	mode := "router"
	live := func() string { return resolveMode("", func() string { return mode }) }
	if live() != "router" {
		t.Fatalf("live() = %q, want router", live())
	}
	mode = "vps"
	if live() != "vps" {
		t.Error("a mode changed in the panel must reach the next call")
	}
}

package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture reads real `sing-box tools networkquality` output captured from the
// shipped binary. Hand-written samples would pin what I THINK it prints; these
// pin what it does — colour codes, \r-drawn progress and all.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(b)
}

func TestParseSpeedTest_RealOutput(t *testing.T) {
	got, err := ParseSpeedTest(fixture(t, "networkquality-ok.txt"))
	if err != nil {
		t.Fatalf("ParseSpeedTest: %v", err)
	}
	// The summary block, not the progress lines above it: those repeat every
	// intermediate reading, and taking one of those would report a figure from
	// the middle of the ramp-up as the result.
	if got.IdleLatencyMs != 209 {
		t.Errorf("IdleLatencyMs = %d, want 209", got.IdleLatencyMs)
	}
	if got.DownloadBps != 122_500_000 {
		t.Errorf("DownloadBps = %d, want 122500000", got.DownloadBps)
	}
	if got.UploadBps != 140_400_000 {
		t.Errorf("UploadBps = %d, want 140400000", got.UploadBps)
	}
	if got.DownloadRPM != 454 || got.UploadRPM != 574 {
		t.Errorf("RPM = %d/%d, want 454/574", got.DownloadRPM, got.UploadRPM)
	}
	if got.DownloadAccuracy != "Medium" || got.UploadAccuracy != "Medium" {
		t.Errorf("capacity accuracy = %q/%q, want Medium/Medium", got.DownloadAccuracy, got.UploadAccuracy)
	}
	if got.DownloadRPMAccuracy != "Low" || got.UploadRPMAccuracy != "Low" {
		t.Errorf("rpm accuracy = %q/%q, want Low/Low", got.DownloadRPMAccuracy, got.UploadRPMAccuracy)
	}
}

// The failure the operator is most likely to hit: testing an outbound that only
// exists in the unapplied draft. The message has to name that, not a stack.
func TestParseSpeedTest_OutboundNotFound(t *testing.T) {
	_, err := ParseSpeedTest(fixture(t, "networkquality-notfound.txt"))
	if err == nil {
		t.Fatal("a run that measured nothing must not parse as a result")
	}
	if !strings.Contains(err.Error(), "outbound not found") {
		t.Fatalf("error = %q, want the binary's own reason", err)
	}
}

// An outbound that exists but cannot reach anything: the tool fails while
// fetching Apple's config, and that reason is the useful one to show.
func TestParseSpeedTest_DeadOutbound(t *testing.T) {
	_, err := ParseSpeedTest(fixture(t, "networkquality-dead.txt"))
	if err == nil {
		t.Fatal("a dead outbound must not parse as a result")
	}
	if strings.Contains(err.Error(), "FATAL") || strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("error = %q, want the reason without the log furniture", err)
	}
	if !strings.Contains(err.Error(), "fetch config") {
		t.Fatalf("error = %q, want the binary's own reason", err)
	}
}

func TestParseSpeedTest_Units(t *testing.T) {
	// bps is decimal here, the way the tool prints it and the way links are sold.
	body := func(dl, ul string) string {
		return "Idle Latency:            10 ms\n" +
			"Download Capacity:       " + dl + "           Accuracy: High\n" +
			"Upload Capacity:         " + ul + "           Accuracy: High\n"
	}
	cases := []struct {
		dl, ul         string
		wantDl, wantUl int64
	}{
		{"1.5 Gbps", "500.0 Mbps", 1_500_000_000, 500_000_000},
		{"850.0 Kbps", "1.0 bps", 850_000, 1},
	}
	for _, c := range cases {
		got, err := ParseSpeedTest(body(c.dl, c.ul))
		if err != nil {
			t.Fatalf("ParseSpeedTest(%s/%s): %v", c.dl, c.ul, err)
		}
		if got.DownloadBps != c.wantDl || got.UploadBps != c.wantUl {
			t.Errorf("%s/%s => %d/%d, want %d/%d", c.dl, c.ul, got.DownloadBps, got.UploadBps, c.wantDl, c.wantUl)
		}
	}
}

// Output with no summary at all must not read as a working 0 Mbps link — that
// is a lie the operator would act on.
func TestParseSpeedTest_EmptyIsAnError(t *testing.T) {
	for _, in := range []string{"", "   \n\n", "==== NETWORK QUALITY TEST ====\nMeasuring idle latency..."} {
		if _, err := ParseSpeedTest(in); err == nil {
			t.Fatalf("ParseSpeedTest(%q) returned a result", in)
		}
	}
}

func TestTrimConfigForSpeedTest(t *testing.T) {
	raw := []byte(`{
	  "log": {"level": "info"},
	  "inbounds": [{"type": "mixed", "tag": "in", "listen_port": 2080}],
	  "outbounds": [{"type": "direct", "tag": "direct"}, {"type": "selector", "tag": "g", "outbounds": ["direct"]}],
	  "endpoints": [{"type": "wireguard", "tag": "wg"}],
	  "dns": {"servers": [{"tag": "sys", "type": "local"}], "final": "sys"},
	  "route": {"final": "direct", "default_domain_resolver": {"server": "sys"},
	            "rules": [{"action": "sniff"}],
	            "rule_set": [{"tag": "rs", "type": "remote", "url": "https://example.com/a.srs"}]},
	  "experimental": {"clash_api": {"external_controller": "127.0.0.1:9090"}}
	}`)
	out, err := TrimConfigForSpeedTest(raw)
	if err != nil {
		t.Fatalf("TrimConfigForSpeedTest: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// Gone: a remote rule-set is re-downloaded per run and can fail a test over
	// a URL that has nothing to do with the outbound being measured.
	route, _ := cfg["route"].(map[string]interface{})
	for _, k := range []string{"rules", "rule_set"} {
		if _, ok := route[k]; ok {
			t.Errorf("route.%s survived the trim", k)
		}
	}
	for _, k := range []string{"inbounds", "experimental"} {
		if _, ok := cfg[k]; ok {
			t.Errorf("%s survived the trim", k)
		}
	}

	// Kept: everything the tag under test may reach through.
	if _, ok := route["default_domain_resolver"]; !ok {
		t.Error("route.default_domain_resolver was dropped; the tool refuses to start without it")
	}
	obs, _ := cfg["outbounds"].([]interface{})
	if len(obs) != 2 {
		t.Errorf("outbounds = %d, want both (a selector's members must survive)", len(obs))
	}
	if _, ok := cfg["endpoints"]; !ok {
		t.Error("endpoints were dropped; a wireguard outbound could not dial")
	}
	if _, ok := cfg["dns"]; !ok {
		t.Error("dns was dropped; the resolver tag would dangle")
	}
}

// A config the panel could not parse must fail as a config error, not as a
// measurement of zero.
func TestTrimConfigForSpeedTest_Invalid(t *testing.T) {
	if _, err := TrimConfigForSpeedTest([]byte("{not json")); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

// The trimmed copy carries every secret the config does, so it must not be
// written to a shared /tmp, and a run that was killed must not leave one behind.
func TestSweepSpeedTestLeftovers(t *testing.T) {
	dir := t.TempDir()
	keep := []string{"config.json", "config.json.bak", "config.json.1700000000.bak"}
	drop := []string{speedTestTempPrefix + "123.json", speedTestTempPrefix + "abc.json"}
	for _, n := range append(append([]string{}, keep...), drop...) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sweepSpeedTestLeftovers(dir)

	for _, n := range keep {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("the sweep took %s, which is not its file", n)
		}
	}
	for _, n := range drop {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			t.Errorf("%s survived the sweep", n)
		}
	}
}

// log.output is a panel setting; left in, the measuring process appends its own
// lines to the service log the monitor shows.
func TestTrimConfigForSpeedTest_DropsLog(t *testing.T) {
	out, err := TrimConfigForSpeedTest([]byte(`{"log":{"level":"info","output":"/var/log/routebox/box.log"},"outbounds":[{"type":"direct","tag":"direct"}]}`))
	if err != nil {
		t.Fatalf("TrimConfigForSpeedTest: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg["log"]; ok {
		t.Error("log survived the trim; the test would write into the service's log file")
	}
}

// 33.9 Mbps is 33_900_000, not one bit less.
func TestParseSpeedTest_RoundsRatherThanTruncates(t *testing.T) {
	got, err := ParseSpeedTest("Download Capacity:       33.9 Mbps           Accuracy: High\n" +
		"Upload Capacity:         0.0 bps           Accuracy: Low\n")
	if err != nil {
		t.Fatalf("ParseSpeedTest: %v", err)
	}
	if got.DownloadBps != 33_900_000 {
		t.Errorf("DownloadBps = %d, want 33900000", got.DownloadBps)
	}
	// A summary that really says zero is a result, not a parse failure.
	if got.UploadBps != 0 {
		t.Errorf("UploadBps = %d, want 0", got.UploadBps)
	}
}

// #91: the AWG server endpoint's listen_port is already held by the running
// process, so the measuring process died on "bind: address already in use"
// before it measured anything. The endpoints themselves must survive — for
// AWG/WireGuard the endpoint IS the outbound.
func TestTrimConfigForSpeedTest_DropsEndpointListenPort(t *testing.T) {
	out, err := TrimConfigForSpeedTest([]byte(`{"endpoints":[
	  {"type":"awg","tag":"awg-server","listen_port":44566,"peers":[{"public_key":"k"}]},
	  {"type":"wireguard","tag":"wg-client","listen_port":51820,"system":true,"name":"wg0","address":["10.0.0.2/32"]}
	]}`))
	if err != nil {
		t.Fatalf("TrimConfigForSpeedTest: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	eps, _ := cfg["endpoints"].([]interface{})
	if len(eps) != 2 {
		t.Fatalf("endpoints = %d, want 2 (a wireguard outbound could not dial without them)", len(eps))
	}
	for _, e := range eps {
		ep, _ := e.(map[string]interface{})
		if _, ok := ep["listen_port"]; ok {
			t.Errorf("listen_port survived the trim on %v; the test binds a port the service holds", ep["tag"])
		}
		// system:true makes sing-box create a real kernel interface and install
		// routes; a second one behind the running service collides on the name.
		if _, ok := ep["system"]; ok {
			t.Errorf("system survived the trim on %v; the test would stand up a second kernel interface", ep["tag"])
		}
		if _, ok := ep["address"]; ep["tag"] == "wg-client" && !ok {
			t.Error("the endpoint's own addresses were dropped; it could no longer dial")
		}
		if _, ok := ep["peers"]; ep["tag"] == "awg-server" && !ok {
			t.Error("peers were dropped along with the port")
		}
	}
}

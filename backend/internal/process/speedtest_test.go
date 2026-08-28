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

package process

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SpeedTest is one `sing-box tools networkquality` run through one outbound:
// Apple's RPM methodology, which is what the fork's binary implements (#13).
//
// Capacities are bits per second so the panel can format them the same way it
// formats everything else. Responsiveness is RPM (round-trips per minute under
// load) — the number that says whether the link buffers badly, which a plain
// Mbps figure never shows. Each measurement carries the binary's own accuracy
// verdict; reporting a Low-accuracy number as if it were solid is how a
// speedtest starts lying.
type SpeedTest struct {
	Outbound string `json:"outbound"`

	IdleLatencyMs int `json:"idle_latency_ms"`

	DownloadBps int64 `json:"download_bps"`
	UploadBps   int64 `json:"upload_bps"`
	DownloadRPM int   `json:"download_rpm"`
	UploadRPM   int   `json:"upload_rpm"`

	DownloadAccuracy    string `json:"download_accuracy"`
	UploadAccuracy      string `json:"upload_accuracy"`
	DownloadRPMAccuracy string `json:"download_rpm_accuracy"`
	UploadRPMAccuracy   string `json:"upload_rpm_accuracy"`

	DurationSec int `json:"duration_sec"`
}

// ErrSpeedTestBusy is returned when a test is already running. Two at once
// share the same uplink and both come back wrong, which is worse than waiting.
var ErrSpeedTestBusy = fmt.Errorf("a speed test is already running")

var speedTestMu sync.Mutex

// SpeedTestMaxRuntime bounds the measurement itself. Apple's default is 20s;
// this is the panel's, chosen so the request finishes inside a browser's
// patience while still leaving the ramp-up time to reach a stable figure.
const SpeedTestMaxRuntime = 12

// RunSpeedTest measures one outbound and returns the parsed summary.
//
// It runs against a TRIMMED copy of the config rather than the file itself. The
// tool builds the whole route before measuring anything, which means every
// remote rule-set is downloaded again per test — and a rule-set whose URL is
// briefly unreachable fails the test for a reason that has nothing to do with
// the outbound being measured. `-o` dials the outbound directly, so no routing
// rule can apply; dropping them costs nothing and removes that whole class of
// failure. Outbounds, endpoints and dns are kept whole: a selector's members, a
// detour chain and the resolver are all reachable from the tag under test.
func (m *Manager) RunSpeedTest(ctx context.Context, configPath, outbound string) (SpeedTest, error) {
	if outbound == "" {
		return SpeedTest{}, fmt.Errorf("outbound is required")
	}
	if !speedTestMu.TryLock() {
		return SpeedTest{}, ErrSpeedTestBusy
	}
	defer speedTestMu.Unlock()

	binary := m.GetBinaryPath()
	if binary == "" {
		return SpeedTest{}, fmt.Errorf("no amnezia-box binary to run the test with")
	}

	// Slack over max-runtime: the tool fetches Apple's config and measures idle
	// latency before the clock it bounds starts.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(SpeedTestMaxRuntime+25)*time.Second)
	defer cancel()

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return SpeedTest{}, fmt.Errorf("read config: %w", err)
	}
	trimmed, err := TrimConfigForSpeedTest(raw)
	if err != nil {
		return SpeedTest{}, err
	}
	tmp, err := os.CreateTemp("", "routebox-speedtest-*.json")
	if err != nil {
		return SpeedTest{}, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(trimmed); err != nil {
		tmp.Close()
		return SpeedTest{}, err
	}
	tmp.Close()

	started := time.Now()
	cmd := exec.CommandContext(ctx, binary, "tools", "networkquality",
		"-c", tmp.Name(), "-o", outbound,
		"--max-runtime", strconv.Itoa(SpeedTestMaxRuntime))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return SpeedTest{}, fmt.Errorf("speed test timed out")
		}
		return SpeedTest{}, fmt.Errorf("%s", speedTestFailure(string(out)))
	}

	res, perr := ParseSpeedTest(string(out))
	if perr != nil {
		return SpeedTest{}, perr
	}
	res.Outbound = outbound
	res.DurationSec = int(time.Since(started).Seconds())
	return res, nil
}

// TrimConfigForSpeedTest strips everything the measurement cannot use: routing
// rules and rule-sets (re-downloaded on every run, and able to fail the test
// over an unrelated URL), inbounds and the Clash API (nothing listens during a
// measurement anyway). What is left is what the outbound needs to dial.
func TrimConfigForSpeedTest(raw []byte) ([]byte, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	delete(cfg, "inbounds")
	delete(cfg, "experimental")
	if route, ok := cfg["route"].(map[string]interface{}); ok {
		delete(route, "rules")
		delete(route, "rule_set")
	}
	return json.Marshal(cfg)
}

var (
	// "Idle Latency:            219 ms"
	reIdle = regexp.MustCompile(`(?m)^Idle Latency:\s+(\d+)\s*ms`)
	// "Download Capacity:       141.0 Mbps           Accuracy: High"
	reCapacity = regexp.MustCompile(`(?m)^(Download|Upload) Capacity:\s+([0-9.]+)\s*([KMG]?bps)(?:\s+Accuracy:\s+(\w+))?`)
	// "Download Responsiveness: 396 RPM              Accuracy: Low"
	reRPM = regexp.MustCompile(`(?m)^(Download|Upload) Responsiveness:\s+(\d+)\s*RPM(?:\s+Accuracy:\s+(\w+))?`)
	// The binary's own failure line, e.g. FATAL[0005] fetch config: … EOF
	reFatal = regexp.MustCompile(`(?:FATAL|ERROR)\[[0-9]+\]\s*(.+)`)
)

// unitScale turns the tool's unit suffix into a multiplier. bps is decimal here
// (Mbps = 10^6), matching how the tool prints and how link speeds are sold.
var unitScale = map[string]float64{"bps": 1, "Kbps": 1e3, "Mbps": 1e6, "Gbps": 1e9}

// ParseSpeedTest reads the summary block `tools networkquality` prints last.
//
// Split out from the run so the shape of that output is pinned by tests without
// a binary, a network, or twelve seconds. The progress lines above the summary
// are overwritten in place with \r and repeat every measured value, so the
// patterns are anchored to line starts and the LAST match wins.
func ParseSpeedTest(output string) (SpeedTest, error) {
	// Progress is drawn with \r; without this the summary is not at a line start.
	text := strings.ReplaceAll(output, "\r", "\n")
	text = stripANSI(text)

	var res SpeedTest
	if m := reIdle.FindAllStringSubmatch(text, -1); len(m) > 0 {
		res.IdleLatencyMs, _ = strconv.Atoi(m[len(m)-1][1])
	}

	seen := 0
	for _, m := range reCapacity.FindAllStringSubmatch(text, -1) {
		val, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		scale, ok := unitScale[m[3]]
		if !ok {
			continue
		}
		bps := int64(val * scale)
		if m[1] == "Download" {
			res.DownloadBps, res.DownloadAccuracy = bps, m[4]
		} else {
			res.UploadBps, res.UploadAccuracy = bps, m[4]
		}
		seen++
	}
	for _, m := range reRPM.FindAllStringSubmatch(text, -1) {
		rpm, _ := strconv.Atoi(m[2])
		if m[1] == "Download" {
			res.DownloadRPM, res.DownloadRPMAccuracy = rpm, m[3]
		} else {
			res.UploadRPM, res.UploadRPMAccuracy = rpm, m[3]
		}
	}

	// A run that printed no capacity at all measured nothing; returning zeroes
	// would render as a working 0 Mbps link.
	if seen == 0 {
		return SpeedTest{}, fmt.Errorf("%s", speedTestFailure(output))
	}
	return res, nil
}

// speedTestFailure reduces the tool's output to the one line worth showing.
// Its own FATAL carries the real cause ("outbound not found", a dial error);
// without one, the last non-empty line is still better than the whole log.
func speedTestFailure(output string) string {
	text := stripANSI(strings.ReplaceAll(output, "\r", "\n"))
	if m := reFatal.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return "the speed test produced no output"
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes the colour codes the tool writes when it thinks it has a
// terminal; they land in the middle of the very words being matched.
func stripANSI(s string) string { return reANSI.ReplaceAllString(s, "") }

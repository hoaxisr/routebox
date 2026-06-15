package config

import (
	"os/exec"
	"testing"
)

// TestRejectRule_PassesSingboxCheck writes a minimal-but-valid sing-box config,
// confirms the BASE passes `<binary> check`, then syncs the managed reject rule
// and re-checks. Base-first: if the base fails, the fixture is wrong (SKIP, not
// our fault); if only the post-inject check fails, the reject rule shape/
// placement is invalid and would down the live VPN. Skips when no binary on PATH.
func TestRejectRule_PassesSingboxCheck(t *testing.T) {
	binary := ""
	for _, b := range []string{"amnezia-box", "sing-box"} {
		if _, err := exec.LookPath(b); err == nil {
			binary = b
			break
		}
	}
	if binary == "" {
		t.Skip("no amnezia-box/sing-box binary on PATH; live check covered by e2e (Task 18)")
	}

	// A minimal valid config: one mixed inbound + a direct outbound + a benign
	// route rule, so the only thing under test is route.rules[0] (the managed rule).
	base := `{
	  "log": {"level": "error"},
	  "inbounds": [{"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":12345}],
	  "outbounds": [{"type":"direct","tag":"direct"}],
	  "route": {"rules": [{"protocol":["dns"],"action":"hijack-dns"}]}
	}`
	p := writeV2Cfg(t, base)

	// Base-first: the fixture itself must pass — else it is env-specific, not us.
	if out, err := exec.Command(binary, "check", "-c", p).CombinedOutput(); err != nil {
		t.Skipf("base fixture does not pass %s check (env-specific): %s", binary, out)
	}

	m, err := NewManager(p)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if changed, err := m.SyncRejectRuleActive([]string{"alice", "bob"}); err != nil || !changed {
		t.Fatalf("sync changed=%v err=%v, want true/nil", changed, err)
	}

	out, err := exec.Command(binary, "check", "-c", p).CombinedOutput()
	if err != nil {
		t.Fatalf("config WITH managed reject rule FAILED %s check (would break live VPN):\n%s", binary, out)
	}
}

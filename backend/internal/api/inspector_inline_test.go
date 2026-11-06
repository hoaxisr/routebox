package api

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRuleSetMatchInline(t *testing.T) {
	if findSingboxBinary() == "" {
		t.Skip("no amnezia-box/sing-box binary on PATH")
	}
	rs := map[string]interface{}{"tag": "youtube", "type": "inline", "rules": []interface{}{
		map[string]interface{}{"domain_suffix": []interface{}{"youtube.com"}},
	}}
	for host, want := range map[string]bool{"www.youtube.com": true, "example.org": false} {
		got, err := checkRuleSetMatch(host, rs, "")
		if err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		if got != want {
			t.Errorf("%s: got %v, want %v", host, got, want)
		}
	}
}

// The temp file goes to os.TempDir; a router's /tmp must not fill up.
func TestCheckRuleSetMatchInlineLeavesNoTempFile(t *testing.T) {
	if findSingboxBinary() == "" {
		t.Skip("no amnezia-box/sing-box binary on PATH")
	}
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	rs := map[string]interface{}{"type": "inline", "rules": []interface{}{
		map[string]interface{}{"domain_suffix": []interface{}{"youtube.com"}},
	}}
	if _, err := checkRuleSetMatch("youtube.com", rs, ""); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
}

func TestCheckRuleMatchInlineRoutesThroughItsRule(t *testing.T) {
	if findSingboxBinary() == "" {
		t.Skip("no amnezia-box/sing-box binary on PATH")
	}
	rs := map[string]map[string]interface{}{"youtube": {"tag": "youtube", "type": "inline", "rules": []interface{}{
		map[string]interface{}{"domain_suffix": []interface{}{"youtube.com"}},
	}}}
	rule := map[string]interface{}{"rule_set": []interface{}{"youtube"}, "outbound": "yt-out"}
	got := checkRuleMatch("youtube.com", "domain", rule, rs, "")
	if !got.Matched || got.Outbound != "yt-out" {
		t.Fatalf("%+v", got)
	}
	got = checkRuleMatch("example.org", "domain", rule, rs, "")
	if got.Matched || strings.Contains(got.Reason, "()") {
		t.Fatalf("%+v", got)
	}
}

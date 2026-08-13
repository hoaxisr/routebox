package config

import (
	"os/exec"
	"strings"
	"testing"
)

// Everything here defends one fact about sing-box that no option doc states:
// from the SECOND dns.servers entry on, a config without
// route.default_domain_resolver is FATAL at startup (see domain_resolver.go).
// The panel hands operators that second server itself — the DNS fallback needs
// one — so each edit below is a way to walk out of a working config into a box
// that will not come back up.

func loadResolverCfg(t *testing.T, body string) *Manager {
	t.Helper()
	m, err := NewManager(writeV2Cfg(t, body))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func resolverOf(m *Manager) string { return domainResolverTag(m.getRoute()) }

func TestDomainResolverPinnedWhenSecondServerAppears(t *testing.T) {
	const oneServer = `{"outbounds":[{"type":"direct","tag":"direct"}],
		"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"}]}}`

	t.Run("second server pins the one already in use", func(t *testing.T) {
		m := loadResolverCfg(t, oneServer)
		if err := m.CreateDnsServer(map[string]interface{}{"tag": "dns-b", "type": "udp", "server": "8.8.8.8"}); err != nil {
			t.Fatalf("CreateDnsServer: %v", err)
		}
		if got := resolverOf(m); got != "dns-a" {
			t.Fatalf("default_domain_resolver = %q, want dns-a — the box will not start", got)
		}
	})

	t.Run("dns.final wins over position", func(t *testing.T) {
		m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}],
			"dns":{"final":"dns-x","servers":[
				{"tag":"dns-a","type":"udp","server":"1.1.1.1"},
				{"tag":"dns-x","type":"udp","server":"9.9.9.9"}]}}`)
		if err := m.CreateDnsServer(map[string]interface{}{"tag": "dns-c", "type": "udp", "server": "8.8.8.8"}); err != nil {
			t.Fatalf("CreateDnsServer: %v", err)
		}
		if got := resolverOf(m); got != "dns-x" {
			t.Fatalf("default_domain_resolver = %q, want dns-x (dns.final)", got)
		}
	})

	t.Run("the first server needs no pin", func(t *testing.T) {
		m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}]}`)
		if err := m.CreateDnsServer(map[string]interface{}{"tag": "dns-a", "type": "udp", "server": "1.1.1.1"}); err != nil {
			t.Fatalf("CreateDnsServer: %v", err)
		}
		if got := resolverOf(m); got != "" {
			t.Fatalf("default_domain_resolver = %q, want none for a single server", got)
		}
	})

	t.Run("an existing pin is left alone", func(t *testing.T) {
		m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}],
			"route":{"default_domain_resolver":"dns-a"},
			"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"},{"tag":"dns-b","type":"udp","server":"8.8.8.8"}]}}`)
		if err := m.CreateDnsServer(map[string]interface{}{"tag": "dns-c", "type": "udp", "server": "9.9.9.9"}); err != nil {
			t.Fatalf("CreateDnsServer: %v", err)
		}
		if got := resolverOf(m); got != "dns-a" {
			t.Fatalf("default_domain_resolver = %q, want the operator's dns-a", got)
		}
	})
}

// #68: the reporter cleared this field and could then apply nothing at all —
// every attempt died on sing-box's FATAL, including the one that would have put
// the value back.
func TestDomainResolverCannotBeClearedPastTwoServers(t *testing.T) {
	two := `{"outbounds":[{"type":"direct","tag":"direct"}],
		"route":{"default_domain_resolver":"dns-a"},
		"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"},{"tag":"dns-b","type":"udp","server":"8.8.8.8"}]}}`

	for _, cleared := range []interface{}{"", nil} {
		m := loadResolverCfg(t, two)
		err := m.UpdateRouteSettings(map[string]interface{}{"default_domain_resolver": cleared})
		if err == nil {
			t.Fatalf("clearing with %#v was accepted; the box would not restart", cleared)
		}
		if !strings.Contains(err.Error(), "two or more DNS servers") {
			t.Fatalf("error does not say why: %v", err)
		}
		if got := resolverOf(m); got != "dns-a" {
			t.Fatalf("default_domain_resolver = %q, want it untouched", got)
		}
	}

	t.Run("one server is still free to clear", func(t *testing.T) {
		m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}],
			"route":{"default_domain_resolver":"dns-a"},
			"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"}]}}`)
		if err := m.UpdateRouteSettings(map[string]interface{}{"default_domain_resolver": ""}); err != nil {
			t.Fatalf("UpdateRouteSettings: %v", err)
		}
		if got := resolverOf(m); got != "" {
			t.Fatalf("default_domain_resolver = %q, want it gone", got)
		}
	})
}

// A pin naming a server that is gone is its own FATAL ("default domain resolver
// not found"), so neither rename nor delete may leave one behind.
func TestDomainResolverFollowsItsServer(t *testing.T) {
	const twoServers = `{"outbounds":[{"type":"direct","tag":"direct"}],
		"route":{"default_domain_resolver":"dns-a"},
		"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"},{"tag":"dns-b","type":"udp","server":"8.8.8.8"}]}}`

	t.Run("rename repoints it", func(t *testing.T) {
		m := loadResolverCfg(t, twoServers)
		if err := m.UpdateDnsServer("dns-a", map[string]interface{}{"tag": "dns-renamed", "type": "udp", "server": "1.1.1.1"}); err != nil {
			t.Fatalf("UpdateDnsServer: %v", err)
		}
		if got := resolverOf(m); got != "dns-renamed" {
			t.Fatalf("default_domain_resolver = %q, want dns-renamed", got)
		}
	})

	t.Run("rename repoints the object form too", func(t *testing.T) {
		m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}],
			"route":{"default_domain_resolver":{"server":"dns-a","strategy":"prefer_ipv4"}},
			"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"},{"tag":"dns-b","type":"udp","server":"8.8.8.8"}]}}`)
		if err := m.UpdateDnsServer("dns-a", map[string]interface{}{"tag": "dns-renamed", "type": "udp", "server": "1.1.1.1"}); err != nil {
			t.Fatalf("UpdateDnsServer: %v", err)
		}
		obj, _ := m.getRoute()["default_domain_resolver"].(map[string]interface{})
		if obj["server"] != "dns-renamed" || obj["strategy"] != "prefer_ipv4" {
			t.Fatalf("default_domain_resolver = %#v, want the object repointed and its strategy kept", obj)
		}
	})

	t.Run("deleting the pinned server is refused while two remain", func(t *testing.T) {
		m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}],
			"route":{"default_domain_resolver":"dns-a"},
			"dns":{"servers":[
				{"tag":"dns-a","type":"udp","server":"1.1.1.1"},
				{"tag":"dns-b","type":"udp","server":"8.8.8.8"},
				{"tag":"dns-c","type":"udp","server":"9.9.9.9"}]}}`)
		if err := m.DeleteDnsServer("dns-a"); err == nil {
			t.Fatal("delete accepted; default_domain_resolver would name a server that is gone")
		}
	})

	t.Run("dropping back to one server takes the pin with it", func(t *testing.T) {
		m := loadResolverCfg(t, twoServers)
		if err := m.DeleteDnsServer("dns-a"); err != nil {
			t.Fatalf("DeleteDnsServer: %v", err)
		}
		if got := resolverOf(m); got != "" {
			t.Fatalf("default_domain_resolver = %q, want it removed with its server", got)
		}
	})
}

func TestMissingDomainResolverHint(t *testing.T) {
	raw := []string{"ERROR missing `route.default_domain_resolver` or `domain_resolver` in dial fields is deprecated in sing-box 1.12.0"}
	got := explainCheckErrors(raw)
	if len(got) != 2 || !strings.Contains(got[1], "Default domain resolver") {
		t.Fatalf("explainCheckErrors = %#v, want the raw line plus a plain explanation", got)
	}
	if unrelated := explainCheckErrors([]string{"some other failure"}); len(unrelated) != 1 {
		t.Fatalf("explainCheckErrors = %#v, want unrelated errors untouched", unrelated)
	}
}

// The claim the rest of this file is built on, checked against the binary
// itself: two DNS servers and no pin is a startup failure, one server is not,
// and the pin fixes it. Skips without a binary — the CI job installs one.
func TestDomainResolverThreshold_AgainstBinary(t *testing.T) {
	binary := ""
	for _, b := range []string{"amnezia-box", "sing-box"} {
		if _, err := exec.LookPath(b); err == nil {
			binary = b
			break
		}
	}
	if binary == "" {
		t.Skip("no amnezia-box/sing-box binary on PATH")
	}

	const oneServer = `{"log":{"level":"error"},
		"outbounds":[{"type":"direct","tag":"direct"}],
		"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"}]}}`
	if ok, errs := CheckConfigWith(binary, writeV2Cfg(t, oneServer)); !ok {
		t.Skipf("base fixture does not pass %s check (env-specific): %v", binary, errs)
	}

	m := loadResolverCfg(t, oneServer)
	if err := m.CreateDnsServer(map[string]interface{}{"tag": "dns-b", "type": "udp", "server": "8.8.8.8"}); err != nil {
		t.Fatalf("CreateDnsServer: %v", err)
	}
	if err := m.ApplyDraft(); err != nil {
		t.Fatalf("ApplyDraft: %v", err)
	}
	if ok, errs := CheckConfigWith(binary, m.GetPath()); !ok {
		t.Fatalf("adding a second DNS server produced a config %s refuses: %v", binary, errs)
	}

	// And without the pin the same config is exactly the failure from #68.
	route := m.getRoute()
	delete(route, domainResolverKey)
	if err := m.SaveToDisk(); err != nil {
		t.Fatalf("SaveToDisk: %v", err)
	}
	ok, errs := CheckConfigWith(binary, m.GetPath())
	if ok {
		t.Skipf("%s accepts two DNS servers without a resolver — build predates the deprecation, nothing to defend", binary)
	}
	if !strings.Contains(strings.Join(errs, "\n"), "Default domain resolver") {
		t.Fatalf("check failed without the panel's explanation: %v", errs)
	}
}

// A tag that names no DNS server is the same brick from the other side:
// sing-box exits on "default domain resolver not found".
func TestDomainResolverRejectsUnknownServer(t *testing.T) {
	m := loadResolverCfg(t, `{"outbounds":[{"type":"direct","tag":"direct"}],
		"route":{"default_domain_resolver":"dns-a"},
		"dns":{"servers":[{"tag":"dns-a","type":"udp","server":"1.1.1.1"},{"tag":"dns-b","type":"udp","server":"8.8.8.8"}]}}`)
	if err := m.UpdateRouteSettings(map[string]interface{}{"default_domain_resolver": "dns-typo"}); err == nil {
		t.Fatal("a resolver naming no DNS server was accepted")
	}
	if got := resolverOf(m); got != "dns-a" {
		t.Fatalf("default_domain_resolver = %q, want it untouched", got)
	}
}

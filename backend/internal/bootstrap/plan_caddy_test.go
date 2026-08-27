package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func caddyfileOK(t *testing.T, p Params) string {
	t.Helper()
	out, err := PlanCaddyfile(p)
	if err != nil {
		t.Fatalf("PlanCaddyfile: %v", err)
	}
	return out
}

// requireLine asserts on a whole line, not a substring: "probe_resistance"
// found anywhere would also match a comment mentioning it.
func requireLine(t *testing.T, got, want string) {
	t.Helper()
	for _, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Fatalf("Caddyfile has no line %q:\n%s", want, got)
}

func TestPlanCaddyfileServesStubAtRoot(t *testing.T) {
	out := caddyfileOK(t, fixture())
	requireLine(t, out, "root * /var/lib/routebox/stub")
	requireLine(t, out, "file_server")
}

func TestPlanCaddyfileServesPanelAtSecretPath(t *testing.T) {
	out := caddyfileOK(t, fixture())
	requireLine(t, out, "route /panel-9c2f1a* {")
	requireLine(t, out, "uri strip_prefix /panel-9c2f1a")
	requireLine(t, out, "reverse_proxy 127.0.0.1:8080")
}

// The Caddyfile and the sing-box config are two halves of one path fork: if the
// path or the port drifts on either side, the inbound stops being reachable.
// So the expectation is built from PlanSingbox's own output, not from literals.
func TestPlanCaddyfilePathsMatchInbounds(t *testing.T) {
	p := fixture()
	inbounds := inboundsByTag(t, planOK(t, p))

	ws := inbounds[TagTrojanWS]
	wsPath := ws["transport"].(map[string]interface{})["path"].(string)
	wsPort := int(ws["listen_port"].(float64))

	grpc := inbounds[TagVlessGRPC]
	grpcName := grpc["transport"].(map[string]interface{})["service_name"].(string)
	grpcPort := int(grpc["listen_port"].(float64))

	out := caddyfileOK(t, p)
	requireLine(t, out, fmt.Sprintf("reverse_proxy %s* 127.0.0.1:%d", wsPath, wsPort))
	requireLine(t, out, fmt.Sprintf("reverse_proxy /%s/* h2c://127.0.0.1:%d", grpcName, grpcPort))
}

func TestPlanCaddyfileNaiveIsProbeResistant(t *testing.T) {
	out := caddyfileOK(t, fixture())
	requireLine(t, out, "order forward_proxy before file_server")
	requireLine(t, out, "forward_proxy {")
	requireLine(t, out, "probe_resistance")
}

// naive's users are not a second user list — they are the inbound user list,
// rendered. Same source, so the check reads the source.
func TestPlanCaddyfileNaiveUsersMatchInbounds(t *testing.T) {
	p := fixture()
	inbounds := inboundsByTag(t, planOK(t, p))
	users := inbounds[TagTrojanWS]["users"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("expected exactly one inbound user, got %d", len(users))
	}
	u := users[0].(map[string]interface{})

	out := caddyfileOK(t, p)
	requireLine(t, out, fmt.Sprintf("basic_auth %s %s", u["name"], u["password"]))
}

func TestPlanCaddyfileUsesACME(t *testing.T) {
	out := caddyfileOK(t, fixture())
	requireLine(t, out, "issuer acme {")
	// 443/TCP belongs to sing-box (ADR 0001), so TLS-ALPN-01 can never succeed.
	requireLine(t, out, "disable_tlsalpn_challenge")
	if strings.Contains(out, "staging") {
		t.Fatalf("production plan points at the staging CA:\n%s", out)
	}
}

func TestPlanCaddyfileCanSwitchToStagingCA(t *testing.T) {
	p := fixture()
	p.ACME.Staging = true
	requireLine(t, caddyfileOK(t, p), "dir https://acme-staging-v02.api.letsencrypt.org/directory")
}

func TestPlanCaddyfileIsDeterministic(t *testing.T) {
	a := caddyfileOK(t, fixture())
	b := caddyfileOK(t, fixture())
	if a != b {
		t.Fatalf("same input gave different bytes:\n%s\n---\n%s", a, b)
	}
}

// A password is operator-supplied text, and the Caddyfile is a token grammar:
// unquoted, a space in it silently becomes an extra argument.
func TestPlanCaddyfileQuotesCredentials(t *testing.T) {
	p := fixture()
	p.User.Password = `pa ss"word`
	requireLine(t, caddyfileOK(t, p), `basic_auth owner "pa ss\"word"`)
}

func TestPlanCaddyfileRejectsIncompleteInput(t *testing.T) {
	cases := map[string]func(*Params){
		"no stub root":    func(p *Params) { p.StubRoot = "" },
		"no panel port":   func(p *Params) { p.Ports.Panel = 0 },
		"no panel path":   func(p *Params) { p.Paths.Panel = "" },
		"panel path bare": func(p *Params) { p.Paths.Panel = "panel-9c2f1a" },
		// A path lands in a matcher token, where a space or a brace is grammar.
		"space in path":   func(p *Params) { p.Paths.TrojanWS = "/ws 4f2b8d" },
		"brace in path":   func(p *Params) { p.Paths.VlessGRPC = "grpc{7a1c9e}" },
		"space in domain": func(p *Params) { p.Domain = "media example.com" },
		"space in root":   func(p *Params) { p.StubRoot = "/var/lib/route box" },
	}
	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			p := fixture()
			break_(&p)
			if _, err := PlanCaddyfile(p); err == nil {
				t.Fatal("incomplete input accepted")
			}
		})
	}
}

// The real oracle, when there is one on this machine: Caddy itself — and only a
// Caddy built with the naive forwardproxy fork, since a stock one does not know
// the forward_proxy directive at all.
func TestPlanCaddyfilePassesCaddyValidate(t *testing.T) {
	binary, err := exec.LookPath("caddy")
	if err != nil {
		t.Skip("no caddy binary on PATH")
	}
	modules, err := exec.Command(binary, "list-modules").Output()
	if err != nil || !strings.Contains(string(modules), "forward_proxy") {
		t.Skip("caddy on PATH has no forwardproxy module")
	}
	staging := fixture()
	staging.ACME.Staging = true
	t.Run("acme solvers stay reachable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Caddyfile")
		if err := os.WriteFile(path, []byte(caddyfileOK(t, fixture())), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(binary, "adapt", "--adapter", "caddyfile", "--config", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		// Caddy copies a site's bind host onto the ACME solvers. A bind_host here
		// means the HTTP-01 listener sits on loopback, where Let's Encrypt cannot
		// reach it — valid syntax, no certificate. Only the adapted JSON shows it.
		if strings.Contains(string(out), "bind_host") {
			t.Fatalf("acme challenges are bound to one host, so HTTP-01 cannot be reached:\n%s", out)
		}
	})
	for name, p := range map[string]Params{"production": fixture(), "staging": staging} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Caddyfile")
			if err := os.WriteFile(path, []byte(caddyfileOK(t, p)), 0644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(binary, "validate", "--adapter", "caddyfile", "--config", path).CombinedOutput()
			if err != nil {
				t.Fatalf("caddy validate rejected the planned Caddyfile: %v\n%s", err, out)
			}
		})
	}
}

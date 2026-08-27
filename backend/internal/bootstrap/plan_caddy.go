package bootstrap

import (
	"fmt"
	"strings"
)

// caddyfileUnsafe are the characters that are grammar rather than data in a
// Caddyfile token. A value carrying one must be quoted; a value that cannot be
// quoted — a matcher, a root — is rejected in validate() instead. One list, so
// the two places cannot drift apart.
const caddyfileUnsafe = " \t\r\n\"\\{}"

// stagingCA is Let's Encrypt's test directory: same protocol, throwaway certs,
// no rate limit worth hitting. The only knob an operator needs while trying an
// install out.
const stagingCA = "https://acme-staging-v02.api.letsencrypt.org/directory"

// PlanCaddyfile builds the Caddyfile dest runs with — the other half of the
// same plan PlanSingbox produces. Everything the front does not fork off to an
// inbound lands here: the stub site at the root, the panel and the transport
// inbounds on their secret paths, and naive, which dest itself serves through
// the forwardproxy fork (ADR 0002).
//
// naive's users are not a second user list. The basic_auth block below renders
// the same User the inbounds carry, so the panel stays the only owner of users
// and Caddy is only an executor.
func PlanCaddyfile(p Params) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}

	var b strings.Builder
	w := func(format string, args ...interface{}) {
		fmt.Fprintf(&b, format+"\n", args...)
	}

	// forward_proxy is not a standard directive, so it has no place in Caddy's
	// directive order until we give it one.
	w("{")
	w("\torder forward_proxy before file_server")
	w("}")
	w("")

	// No bind directive, deliberately: Caddy copies a site's bind host onto the
	// ACME solvers too, so binding dest to loopback would put the HTTP-01
	// listener on 127.0.0.1:80, where Let's Encrypt cannot reach it and the
	// certificate the front borrows never issues. Caddy validate does not catch
	// that — the syntax is fine, only the issuance fails, in production.
	// ponytail: the dest port is therefore reachable from outside; keeping it
	// private is the installer's firewall rule, not a line in this file.
	w("%s:%d {", p.Domain, p.DestPort)
	w("")
	w("\ttls {")
	w("\t\tissuer acme {")
	if p.ACME.Staging {
		w("\t\t\tdir %s", stagingCA)
	}
	// TLS-ALPN-01 answers on 443/TCP, which belongs to sing-box (ADR 0001), so
	// it could never succeed here. HTTP-01 on the free :80 is the only option.
	w("\t\t\tdisable_tlsalpn_challenge")
	w("\t\t}")
	w("\t}")
	w("")

	// One route, so the order below is the order that runs: naive first — it is
	// recognised by the request being a proxy request, not by a path — then the
	// secret paths, then the stub site as the catch-all.
	w("\troute {")
	w("\t\tforward_proxy {")
	w("\t\t\tbasic_auth %s %s", quote(p.User.Name), quote(p.User.Password))
	w("\t\t\thide_ip")
	w("\t\t\thide_via")
	// Wrong password must not answer 407: with probe_resistance dest keeps
	// looking like the ordinary web server it also is.
	w("\t\t\tprobe_resistance")
	w("\t\t}")
	w("")
	w("\t\treverse_proxy %s* %s:%d", p.Paths.TrojanWS, loopback, p.Ports.TrojanWS)
	// sing-box's grpc inbound speaks h2c in the clear behind dest, and its
	// service name is the first path segment.
	w("\t\treverse_proxy /%s/* h2c://%s:%d", p.Paths.VlessGRPC, loopback, p.Ports.VlessGRPC)
	w("")
	w("\t\troute %s* {", p.Paths.Panel)
	w("\t\t\turi strip_prefix %s", p.Paths.Panel)
	w("\t\t\treverse_proxy %s:%d", loopback, p.Ports.Panel)
	w("\t\t}")
	w("")
	w("\t\troot * %s", p.StubRoot)
	w("\t\tfile_server")
	w("\t}")
	w("}")

	return b.String(), nil
}

// quote renders a value as one Caddyfile token. Credentials are operator text:
// unquoted, a space in a password silently becomes an extra argument to
// basic_auth and the account stops working with no error anywhere.
func quote(s string) string {
	if !strings.ContainsAny(s, caddyfileUnsafe) {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

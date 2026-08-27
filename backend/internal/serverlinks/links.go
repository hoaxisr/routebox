package serverlinks

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"routebox/backend/internal/util"
)

// defaultFingerprint is the uTLS fingerprint advertised to clients in TLS/Reality
// share links. The Reality server does not store a client fingerprint, so a
// sensible default is emitted.
const defaultFingerprint = "chrome"

// PublicAddr is the address clients dial to reach this server from outside.
// Neither field is present in the inbound config, so both come from the caller.
// Deliberately NOT named Endpoint: CONTEXT.md reserves that word for the
// AWG/WireGuard interface entity, which this is not.
//
// Port fronts inbounds that bind a loopback address: those are unreachable at
// their own listen_port by definition, so their share links must carry this one
// instead. Zero means nothing fronts them — see clientPort.
type PublicAddr struct {
	Host string
	Port int
}

// ErrNoFront reports that an inbound is only reachable through a front whose
// port nobody configured. Callers rendering a whole subscription treat it as
// configuration policy (skip the node quietly) rather than as a malformed
// inbound worth logging on every request to a public endpoint.
var ErrNoFront = errors.New("inbound listens on loopback but no public front port is configured")

// BuildShareLink turns a server inbound plus one of its users into a client
// share-link URI. It is the inverse of the subscription link parsers.
func BuildShareLink(inbound, user map[string]interface{}, ep PublicAddr) (string, error) {
	host := ep.Host
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	typ, _ := inbound["type"].(string)
	behindFront := listensOnLoopback(inbound)
	// mieru may bind ONLY ranges (listen_ports) and no single port, so for it an
	// absent listen_port is not automatically a broken inbound (#37).
	mieruRanges := typ == "mieru" && len(listOfStrings(inbound["listen_ports"])) > 0
	// A front maps one external port onto one internal one; a range has no such
	// image. Emitting the range anyway leaks internal ports into a client link
	// and pairs them with a port the inbound never listened on.
	if behindFront && mieruRanges {
		return "", fmt.Errorf("inbound listens on loopback with port ranges: a front cannot be mapped onto a range")
	}
	port, err := clientPort(inbound, behindFront, ep)
	if err != nil {
		return "", err
	}
	if port == 0 && !mieruRanges {
		return "", fmt.Errorf("inbound has no listen_port")
	}
	switch typ {
	case "vless":
		return buildVless(inbound, user, host, port)
	case "naive":
		return buildNaive(inbound, user, host, port)
	case "hysteria2":
		return buildHysteria2(inbound, user, host, port)
	case "trojan":
		return buildTrojan(inbound, user, host, port)
	case "mieru":
		return buildMieru(inbound, user, host, port)
	default:
		return "", fmt.Errorf("unsupported inbound type for share link: %q", typ)
	}
}

func buildVless(inbound, user map[string]interface{}, host string, port int) (string, error) {
	uuid, _ := user["uuid"].(string)
	if uuid == "" {
		return "", fmt.Errorf("vless user has no uuid")
	}
	q, err := inboundTLSParams(inbound, host)
	if err != nil {
		return "", err
	}
	tp := transportParams(inbound)
	// flow (xtls-rprx-vision) is only valid on raw transport: emit it only when
	// transportParams produced no type (raw/absent/unknown). Tightest gate.
	isRaw := tp.Get("type") == ""
	if isRaw {
		if flow, _ := user["flow"].(string); flow != "" {
			q.Set("flow", flow)
		}
	}
	mergeValues(q, tp)

	suffix := ""
	if enc := q.Encode(); enc != "" {
		suffix = "?" + enc
	}
	return fmt.Sprintf("vless://%s@%s%s#%s",
		uuid, hostPort(host, port), suffix, url.PathEscape(remarkOf(inbound, user, "VLESS"))), nil
}

// --- shared helpers ---

// hostPort joins a host and port, bracketing IPv6 literals per RFC 3986.
func hostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// clientPort resolves the port a client must dial. For an inbound reachable
// from outside that is its own listen_port, which is every inbound of every
// installation that predates the loopback-fronted layout.
//
// For a loopback-bound inbound the listen_port is meaningless to a client, so
// the front's port is used. With no front configured there is no dialable port
// at all: returning the loopback one would hand out a link that cannot work and
// looks correct, so this fails instead and lets the caller skip the binding.
func clientPort(inbound map[string]interface{}, behindFront bool, ep PublicAddr) (int, error) {
	if !behindFront {
		return portOf(inbound), nil
	}
	if ep.Port == 0 {
		return 0, ErrNoFront
	}
	return ep.Port, nil
}

// listensOnLoopback reports whether the inbound binds an address that cannot be
// reached from another host — the sole marker of "behind the front". The
// predicate itself lives in util, shared with the config validator: if the two
// disagreed, the panel would refuse to save a config whose links it already
// rewrites as fronted.
func listensOnLoopback(inbound map[string]interface{}) bool {
	listen, _ := inbound["listen"].(string)
	return util.IsLoopbackListen(listen)
}

func portOf(m map[string]interface{}) int {
	switch v := m["listen_port"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// intOf reads a JSON number that may have decoded as float64 or int.
func intOf(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func mapOf(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// firstShortID returns the first non-empty Reality short_id. sing-box stores
// short_id as a JSON array ([]interface{} after decode); manual/legacy configs
// may use a bare string. Both are handled so the client always receives a sid.
func firstShortID(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []interface{}:
		for _, e := range s {
			if str, _ := e.(string); str != "" {
				return str
			}
		}
	case []string:
		for _, str := range s {
			if str != "" {
				return str
			}
		}
	}
	return ""
}

func isEnabled(m map[string]interface{}) bool {
	if m == nil {
		return false
	}
	b, _ := m["enabled"].(bool)
	return b
}

// sniOf resolves the client SNI: explicit server_name, else ACME domain, else host.
func sniOf(tls map[string]interface{}, host string) string {
	if tls != nil {
		if sn, _ := tls["server_name"].(string); sn != "" {
			return sn
		}
		if acme := mapOf(tls["acme"]); acme != nil {
			if d, _ := acme["domain"].(string); d != "" {
				return d
			}
		}
	}
	return host
}

// tlsParams builds the shared security/reality/tls query params for vless and
// trojan share links: reality → security=reality&pbk&sid&sni&fp ; plain TLS →
// security=tls&sni&fp. pbk is derived from the server reality private_key; sid
// is the first short_id. Returns an error only if Reality key derivation fails.
func tlsParams(tls map[string]interface{}, host string) (url.Values, error) {
	q := url.Values{}
	if tls == nil {
		return q, nil
	}
	sni := sniOf(tls, host)
	if reality := mapOf(tls["reality"]); isEnabled(reality) {
		priv, _ := reality["private_key"].(string)
		pub, err := RealityPublicFromPrivate(priv)
		if err != nil {
			return nil, fmt.Errorf("reality public key: %w", err)
		}
		q.Set("security", "reality")
		q.Set("pbk", pub)
		if sid := firstShortID(reality["short_id"]); sid != "" {
			q.Set("sid", sid)
		}
		q.Set("sni", sni)
		q.Set("fp", defaultFingerprint)
	} else if isEnabled(tls) {
		q.Set("security", "tls")
		q.Set("sni", sni)
		q.Set("fp", defaultFingerprint)
	}
	return q, nil
}

// inboundTLSParams is tlsParams plus the one thing an inbound cannot say about
// itself: an inbound behind the 443 front has NO tls block, because dest
// terminates TLS with the real certificate and forwards plaintext over
// loopback. The client still speaks TLS — to the front — so the link must carry
// security=tls and the front's SNI. Emitting the inbound's bare plaintext here
// produced a link that looked right and died in the handshake.
//
// Only the absence of TLS is filled in: an inbound that does declare TLS or
// Reality keeps exactly what it declared.
func inboundTLSParams(inbound map[string]interface{}, host string) (url.Values, error) {
	tls := mapOf(inbound["tls"])
	q, err := tlsParams(tls, host)
	if err != nil {
		return nil, err
	}
	if q.Get("security") == "" && listensOnLoopback(inbound) {
		q.Set("security", "tls")
		q.Set("sni", sniOf(tls, host))
		q.Set("fp", defaultFingerprint)
	}
	return q, nil
}

// transportParams builds the shared transport query params (uniform host= on the
// URL; the importer maps it to headers.Host for ws). raw/absent/unknown → empty.
//
//	ws          → type=ws&path=&host=   (host source: transport.headers.Host)
//	httpupgrade → type=httpupgrade&path=&host=  (host source: top-level transport.host)
//	xhttp       → type=xhttp&path=&host=  (host source: top-level transport.host)
//	grpc        → type=grpc&serviceName=
func transportParams(inbound map[string]interface{}) url.Values {
	q := url.Values{}
	tr := mapOf(inbound["transport"])
	if tr == nil {
		return q
	}
	switch t, _ := tr["type"].(string); t {
	case "ws":
		q.Set("type", "ws")
		if p, _ := tr["path"].(string); p != "" {
			q.Set("path", p)
		}
		if h := wsHostOf(tr); h != "" {
			q.Set("host", h)
		}
	case "httpupgrade":
		q.Set("type", "httpupgrade")
		if p, _ := tr["path"].(string); p != "" {
			q.Set("path", p)
		}
		if h, _ := tr["host"].(string); h != "" {
			q.Set("host", h)
		}
	case "xhttp":
		q.Set("type", "xhttp")
		if p, _ := tr["path"].(string); p != "" {
			q.Set("path", p)
		}
		if h, _ := tr["host"].(string); h != "" {
			q.Set("host", h)
		}
	case "grpc":
		q.Set("type", "grpc")
		if sn, _ := tr["service_name"].(string); sn != "" {
			q.Set("serviceName", sn)
		}
	}
	return q
}

// wsHostOf reads the ws transport Host from headers.Host (host matrix).
func wsHostOf(tr map[string]interface{}) string {
	headers := mapOf(tr["headers"])
	if headers == nil {
		return ""
	}
	h, _ := headers["Host"].(string)
	return h
}

// mergeValues copies all keys from src into dst.
func mergeValues(dst, src url.Values) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func nameOf(user map[string]interface{}, fallback string) string {
	for _, k := range []string{"name", "username"} {
		if s, _ := user[k].(string); s != "" {
			return s
		}
	}
	return fallback
}

// remarkOf builds the share-link '#' label as "<name> · <inbound-tag>" (3x-ui
// style), so a user's nodes across multiple inbounds are distinctly named in
// client apps. With an empty inbound tag it falls back to the name alone. The
// returned value is the RAW remark; callers wrap it in url.PathEscape.
func remarkOf(inbound, user map[string]interface{}, fallback string) string {
	name := nameOf(user, fallback)
	if tag, _ := inbound["tag"].(string); tag != "" {
		return name + " · " + tag
	}
	return name
}

func buildTrojan(inbound, user map[string]interface{}, host string, port int) (string, error) {
	password, _ := user["password"].(string)
	if password == "" {
		return "", fmt.Errorf("trojan user has no password")
	}
	q, err := inboundTLSParams(inbound, host)
	if err != nil {
		return "", err
	}
	mergeValues(q, transportParams(inbound))
	suffix := ""
	if enc := q.Encode(); enc != "" {
		suffix = "?" + enc
	}
	return fmt.Sprintf("trojan://%s@%s%s#%s",
		url.User(password).String(), hostPort(host, port), suffix, url.PathEscape(remarkOf(inbound, user, "Trojan"))), nil
}

func buildNaive(inbound, user map[string]interface{}, host string, port int) (string, error) {
	username, _ := user["username"].(string)
	password, _ := user["password"].(string)
	if username == "" {
		return "", fmt.Errorf("naive user has no username")
	}
	userinfo := url.UserPassword(username, password).String()
	return fmt.Sprintf("naive+https://%s@%s#%s",
		userinfo, hostPort(host, port), url.PathEscape(remarkOf(inbound, user, "NaiveProxy"))), nil
}

// buildMieru emits a mierus:// link for a mieru inbound user. Unlike naive the
// port lives in the query (?port=), NOT the authority — the fork's mierus://
// grammar. IPv6 hosts are bracketed manually (no JoinHostPort, which would add
// a port). No multiplexing (inbound has none); traffic-pattern only if set.
// listOfStrings returns the non-empty string elements of a JSON array value.
func listOfStrings(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, raw := range arr {
		if s, ok := raw.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func buildMieru(inbound, user map[string]interface{}, host string, port int) (string, error) {
	name, _ := user["name"].(string)
	password, _ := user["password"].(string)
	if name == "" {
		return "", fmt.Errorf("mieru user has no name")
	}
	// Guard empty password (parity with buildTrojan/buildHysteria2): an empty
	// password would emit "mierus://alice:@host?..." which the client-side
	// parser hard-rejects. Never echo the password value in the error.
	if password == "" {
		return "", fmt.Errorf("mieru user %q has no password", name)
	}
	transport, _ := inbound["transport"].(string)
	if transport == "" {
		transport = "TCP"
	}

	authHost := host
	if strings.Contains(host, ":") { // IPv6 literal → bracket, still no port
		authHost = "[" + host + "]"
	}

	q := url.Values{}
	q.Set("profile", remarkOf(inbound, user, "Mieru"))
	// Ports: the single listen_port (when set) followed by every listen_ports
	// range. mieru's own parser (appctl.URLToClientProfile) pairs port[i] with
	// protocol[i] and REJECTS the whole link when the two counts differ — a
	// single broadcast protocol= only worked while there was exactly one port,
	// so adding ranges silently broke every link (#37/#46). One protocol per
	// port; url.Values.Encode keeps each key's values in insertion order.
	addPort := func(spec string) {
		q.Add("port", spec)
		q.Add("protocol", transport)
	}
	if port > 0 {
		addPort(strconv.Itoa(port))
	}
	for _, raw := range listOfStrings(inbound["listen_ports"]) {
		addPort(raw)
	}
	if tp, _ := inbound["traffic_pattern"].(string); tp != "" {
		q.Set("traffic-pattern", tp)
	}

	return fmt.Sprintf("mierus://%s@%s?%s",
		url.UserPassword(name, password).String(), authHost, q.Encode()), nil
}

func buildHysteria2(inbound, user map[string]interface{}, host string, port int) (string, error) {
	password, _ := user["password"].(string)
	if password == "" {
		return "", fmt.Errorf("hysteria2 user has no password")
	}
	q := url.Values{}
	q.Set("sni", sniOf(mapOf(inbound["tls"]), host))
	if obfs := mapOf(inbound["obfs"]); obfs != nil {
		if t, _ := obfs["type"].(string); t != "" {
			q.Set("obfs", t)
			if p, _ := obfs["password"].(string); p != "" {
				q.Set("obfs-password", p)
			}
			// gecko packet sizes have to match on both ends, so the link carries
			// them; salamander has no such fields (#48).
			if t == "gecko" {
				if n := intOf(obfs["min_packet_size"]); n > 0 {
					q.Set("obfs-min-packet-size", strconv.Itoa(n))
				}
				if n := intOf(obfs["max_packet_size"]); n > 0 {
					q.Set("obfs-max-packet-size", strconv.Itoa(n))
				}
			}
		}
	}
	return fmt.Sprintf("hy2://%s@%s?%s#%s",
		url.User(password).String(), hostPort(host, port), q.Encode(), url.PathEscape(remarkOf(inbound, user, "Hysteria2"))), nil
}

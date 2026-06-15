package serverlinks

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// defaultFingerprint is the uTLS fingerprint advertised to clients in TLS/Reality
// share links. The Reality server does not store a client fingerprint, so a
// sensible default is emitted.
const defaultFingerprint = "chrome"

// BuildShareLink turns a server inbound plus one of its users into a client
// share-link URI. host is the public domain/IP of the server (not present in
// the inbound config). It is the inverse of the subscription link parsers.
func BuildShareLink(inbound, user map[string]interface{}, host string) (string, error) {
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	typ, _ := inbound["type"].(string)
	port := portOf(inbound)
	if port == 0 {
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
	default:
		return "", fmt.Errorf("unsupported inbound type for share link: %q", typ)
	}
}

func buildVless(inbound, user map[string]interface{}, host string, port int) (string, error) {
	uuid, _ := user["uuid"].(string)
	if uuid == "" {
		return "", fmt.Errorf("vless user has no uuid")
	}
	q, err := tlsParams(mapOf(inbound["tls"]), host)
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

func portOf(m map[string]interface{}) int {
	switch v := m["listen_port"].(type) {
	case float64:
		return int(v)
	case int:
		return v
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

// transportParams builds the shared transport query params (uniform host= on the
// URL; the importer maps it to headers.Host for ws). raw/absent/unknown → empty.
//
//	ws          → type=ws&path=&host=   (host source: transport.headers.Host)
//	httpupgrade → type=httpupgrade&path=&host=  (host source: top-level transport.host)
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
	q, err := tlsParams(mapOf(inbound["tls"]), host)
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
		}
	}
	return fmt.Sprintf("hy2://%s@%s?%s#%s",
		url.User(password).String(), hostPort(host, port), q.Encode(), url.PathEscape(remarkOf(inbound, user, "Hysteria2"))), nil
}

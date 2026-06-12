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
	default:
		return "", fmt.Errorf("unsupported inbound type for share link: %q", typ)
	}
}

func buildVless(inbound, user map[string]interface{}, host string, port int) (string, error) {
	uuid, _ := user["uuid"].(string)
	if uuid == "" {
		return "", fmt.Errorf("vless user has no uuid")
	}
	q := url.Values{}
	if flow, _ := user["flow"].(string); flow != "" {
		q.Set("flow", flow)
	}

	tls := mapOf(inbound["tls"])
	sni := sniOf(tls, host)
	if reality := mapOf(tls["reality"]); isEnabled(reality) {
		priv, _ := reality["private_key"].(string)
		pub, err := RealityPublicFromPrivate(priv)
		if err != nil {
			return "", fmt.Errorf("reality public key: %w", err)
		}
		q.Set("security", "reality")
		q.Set("pbk", pub)
		if sid, _ := reality["short_id"].(string); sid != "" {
			q.Set("sid", sid)
		}
		q.Set("sni", sni)
		q.Set("fp", defaultFingerprint)
	} else if isEnabled(tls) {
		q.Set("security", "tls")
		q.Set("sni", sni)
		q.Set("fp", defaultFingerprint)
	}

	suffix := ""
	if enc := q.Encode(); enc != "" {
		suffix = "?" + enc
	}
	return fmt.Sprintf("vless://%s@%s%s#%s",
		uuid, hostPort(host, port), suffix, url.PathEscape(nameOf(user, "VLESS"))), nil
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

func nameOf(user map[string]interface{}, fallback string) string {
	for _, k := range []string{"name", "username"} {
		if s, _ := user[k].(string); s != "" {
			return s
		}
	}
	return fallback
}

func buildNaive(inbound, user map[string]interface{}, host string, port int) (string, error) {
	username, _ := user["username"].(string)
	password, _ := user["password"].(string)
	if username == "" {
		return "", fmt.Errorf("naive user has no username")
	}
	userinfo := url.PathEscape(username) + ":" + url.PathEscape(password)
	return fmt.Sprintf("naive+https://%s@%s#%s",
		userinfo, hostPort(host, port), url.PathEscape(nameOf(user, "NaiveProxy"))), nil
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
		url.PathEscape(password), hostPort(host, port), q.Encode(), url.PathEscape(nameOf(user, "Hysteria2"))), nil
}

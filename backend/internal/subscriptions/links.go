package subscriptions

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParsedNode is a single parsed outbound plus its raw display name (from #name).
// Outbound carries no "tag"; merge prefixes and assigns the tag.
type ParsedNode struct {
	Outbound map[string]interface{}
	Name     string
}

// splitHostPort splits "host:port"; supports bracketed IPv6 "[::1]:443".
// ok is false on empty host, missing port, or port not in 1-65535.
func splitHostPort(s string) (host string, port int, ok bool) {
	var portStr string
	if strings.HasPrefix(s, "[") {
		closeIdx := strings.IndexByte(s, ']')
		if closeIdx == -1 || closeIdx+1 >= len(s) || s[closeIdx+1] != ':' {
			return "", 0, false
		}
		host = s[1:closeIdx]
		portStr = s[closeIdx+2:]
	} else {
		colonIdx := strings.LastIndexByte(s, ':')
		if colonIdx == -1 {
			return "", 0, false
		}
		host = s[:colonIdx]
		portStr = s[colonIdx+1:]
	}
	if host == "" {
		return "", 0, false
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return "", 0, false
	}
	return host, p, true
}

// decodeSsUserinfo decodes SIP002 userinfo: std base64, then base64url (re-pad),
// finally percent-decoded plaintext.
func decodeSsUserinfo(userinfo string) (string, error) {
	if b, err := base64.StdEncoding.DecodeString(userinfo); err == nil {
		return string(b), nil
	}
	normalized := strings.NewReplacer("-", "+", "_", "/").Replace(userinfo)
	if pad := (4 - len(normalized)%4) % 4; pad > 0 {
		normalized += strings.Repeat("=", pad)
	}
	if b, err := base64.StdEncoding.DecodeString(normalized); err == nil {
		return string(b), nil
	}
	dec, err := url.PathUnescape(userinfo)
	if err != nil {
		return "", fmt.Errorf("decode userinfo: %w", err)
	}
	return dec, nil
}

// splitName splits "main#name" returning main and the URL-decoded name,
// rejoining extra '#' fragments (matches the TS split('#') semantics).
func splitName(content, fallback string) (main, name string) {
	parts := strings.Split(content, "#")
	if len(parts) > 1 {
		raw := strings.Join(parts[1:], "#")
		if dec, err := url.PathUnescape(raw); err == nil {
			return parts[0], dec
		}
		return parts[0], raw
	}
	return content, fallback
}

func parseVless(uri string) (map[string]interface{}, string, error) {
	if !strings.HasPrefix(uri, "vless://") {
		return nil, "", fmt.Errorf("invalid vless uri")
	}
	mainPart, name := splitName(uri[len("vless://"):], "VLESS")
	atIdx := strings.IndexByte(mainPart, '@')
	if atIdx == -1 {
		return nil, "", fmt.Errorf("vless: missing @")
	}
	uuid := mainPart[:atIdx]
	hostPort, queryString, _ := strings.Cut(mainPart[atIdx+1:], "?")
	host, port, ok := splitHostPort(hostPort)
	if !ok {
		return nil, "", fmt.Errorf("vless: invalid host:port")
	}
	params, _ := url.ParseQuery(queryString)
	ob := map[string]interface{}{"type": "vless", "server": host, "server_port": port, "uuid": uuid}
	if v := params.Get("flow"); v != "" {
		ob["flow"] = v
	}
	security := params.Get("security")
	if security == "tls" || security == "reality" {
		tls := map[string]interface{}{"enabled": true, "server_name": orElse(params.Get("sni"), host)}
		if fp := params.Get("fp"); fp != "" {
			tls["utls"] = map[string]interface{}{"enabled": true, "fingerprint": fp}
		}
		if alpn := params.Get("alpn"); alpn != "" {
			tls["alpn"] = strings.Split(alpn, ",")
		}
		if security == "reality" {
			if pbk := params.Get("pbk"); pbk != "" {
				tls["reality"] = map[string]interface{}{"enabled": true, "public_key": pbk, "short_id": params.Get("sid")}
			}
		}
		ob["tls"] = tls
	}
	switch params.Get("type") {
	case "ws":
		tr := map[string]interface{}{"type": "ws", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["headers"] = map[string]interface{}{"Host": h}
		}
		ob["transport"] = tr
	case "grpc":
		ob["transport"] = map[string]interface{}{"type": "grpc", "service_name": params.Get("serviceName")}
	case "http":
		tr := map[string]interface{}{"type": "http", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["host"] = []string{h}
		}
		ob["transport"] = tr
	case "httpupgrade":
		tr := map[string]interface{}{"type": "httpupgrade", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["host"] = h
		}
		ob["transport"] = tr
	case "xhttp":
		tr := map[string]interface{}{"type": "xhttp", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["host"] = h
		}
		ob["transport"] = tr
	}
	return ob, name, nil
}

func parseTrojan(uri string) (map[string]interface{}, string, error) {
	if !strings.HasPrefix(uri, "trojan://") {
		return nil, "", fmt.Errorf("invalid trojan uri")
	}
	mainPart, name := splitName(uri[len("trojan://"):], "Trojan")
	atIdx := strings.IndexByte(mainPart, '@')
	if atIdx == -1 {
		return nil, "", fmt.Errorf("trojan: missing @")
	}
	password, err := url.PathUnescape(mainPart[:atIdx])
	if err != nil {
		password = mainPart[:atIdx]
	}
	hostPort, queryString, _ := strings.Cut(mainPart[atIdx+1:], "?")
	host, port, ok := splitHostPort(hostPort)
	if !ok {
		return nil, "", fmt.Errorf("trojan: invalid host:port")
	}
	params, _ := url.ParseQuery(queryString)
	ob := map[string]interface{}{"type": "trojan", "server": host, "server_port": port, "password": password}
	security := params.Get("security")
	if security == "tls" || security == "reality" {
		tls := map[string]interface{}{"enabled": true, "server_name": orElse(params.Get("sni"), host)}
		if fp := params.Get("fp"); fp != "" {
			tls["utls"] = map[string]interface{}{"enabled": true, "fingerprint": fp}
		}
		if alpn := params.Get("alpn"); alpn != "" {
			tls["alpn"] = strings.Split(alpn, ",")
		}
		if security == "reality" {
			if pbk := params.Get("pbk"); pbk != "" {
				tls["reality"] = map[string]interface{}{"enabled": true, "public_key": pbk, "short_id": params.Get("sid")}
			}
		}
		ob["tls"] = tls
	}
	switch params.Get("type") {
	case "ws":
		tr := map[string]interface{}{"type": "ws", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["headers"] = map[string]interface{}{"Host": h}
		}
		ob["transport"] = tr
	case "grpc":
		ob["transport"] = map[string]interface{}{"type": "grpc", "service_name": params.Get("serviceName")}
	case "httpupgrade":
		tr := map[string]interface{}{"type": "httpupgrade", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["host"] = h
		}
		ob["transport"] = tr
	case "xhttp":
		tr := map[string]interface{}{"type": "xhttp", "path": orElse(params.Get("path"), "/")}
		if h := params.Get("host"); h != "" {
			tr["host"] = h
		}
		ob["transport"] = tr
	}
	return ob, name, nil
}

func parseHysteria2(uri string) (map[string]interface{}, string, error) {
	var content string
	switch {
	case strings.HasPrefix(uri, "hy2://"):
		content = uri[len("hy2://"):]
	case strings.HasPrefix(uri, "hysteria2://"):
		content = uri[len("hysteria2://"):]
	default:
		return nil, "", fmt.Errorf("invalid hysteria2 uri")
	}
	mainPart, name := splitName(content, "Hysteria2")
	atIdx := strings.IndexByte(mainPart, '@')
	if atIdx == -1 {
		return nil, "", fmt.Errorf("hysteria2: missing @")
	}
	password, err := url.PathUnescape(mainPart[:atIdx])
	if err != nil {
		password = mainPart[:atIdx]
	}
	hostPort, queryString, _ := strings.Cut(mainPart[atIdx+1:], "?")
	host, port, ok := splitHostPort(hostPort)
	if !ok {
		return nil, "", fmt.Errorf("hysteria2: invalid host:port")
	}
	params, _ := url.ParseQuery(queryString)
	ob := map[string]interface{}{
		"type": "hysteria2", "server": host, "server_port": port, "password": password,
		"tls": map[string]interface{}{"enabled": true, "server_name": orElse(params.Get("sni"), host), "insecure": params.Get("insecure") == "1"},
	}
	if obfs := params.Get("obfs"); obfs != "" {
		ob["obfs"] = map[string]interface{}{"type": obfs, "password": params.Get("obfs-password")}
	}
	return ob, name, nil
}

func parseShadowsocks(uri string) (map[string]interface{}, string, error) {
	if !strings.HasPrefix(uri, "ss://") {
		return nil, "", fmt.Errorf("invalid ss uri")
	}
	mainPart, name := splitName(uri[len("ss://"):], "Shadowsocks")
	var host, method, password, plugin, pluginOpts string
	var port int
	atIdx := strings.LastIndexByte(mainPart, '@')
	if atIdx == -1 {
		base := mainPart
		if i := strings.IndexByte(base, '?'); i != -1 {
			base = base[:i]
		}
		if i := strings.IndexByte(base, '/'); i != -1 {
			base = base[:i]
		}
		decBytes, err := base64.StdEncoding.DecodeString(base)
		if err != nil {
			return nil, "", fmt.Errorf("ss: cannot decode base64")
		}
		decoded := string(decBytes)
		dAt := strings.LastIndexByte(decoded, '@')
		if dAt == -1 {
			return nil, "", fmt.Errorf("ss: missing @")
		}
		userinfo := decoded[:dAt]
		cIdx := strings.IndexByte(userinfo, ':')
		if cIdx == -1 {
			return nil, "", fmt.Errorf("ss: missing method:password")
		}
		method, password = userinfo[:cIdx], userinfo[cIdx+1:]
		h, p, ok := splitHostPort(decoded[dAt+1:])
		if !ok {
			return nil, "", fmt.Errorf("ss: invalid host:port")
		}
		host, port = h, p
	} else {
		userinfo := mainPart[:atIdx]
		hostPort, queryString, _ := strings.Cut(mainPart[atIdx+1:], "?")
		h, p, ok := splitHostPort(strings.TrimSuffix(hostPort, "/"))
		if !ok {
			return nil, "", fmt.Errorf("ss: invalid host:port")
		}
		host, port = h, p
		if queryString != "" {
			params, _ := url.ParseQuery(queryString)
			if pl := params.Get("plugin"); pl != "" {
				if semi := strings.IndexByte(pl, ';'); semi != -1 {
					plugin, pluginOpts = pl[:semi], pl[semi+1:]
				} else {
					plugin = pl
				}
			}
		}
		decoded, err := decodeSsUserinfo(userinfo)
		if err != nil {
			return nil, "", err
		}
		cIdx := strings.IndexByte(decoded, ':')
		if cIdx == -1 {
			return nil, "", fmt.Errorf("ss: missing method:password")
		}
		method, password = decoded[:cIdx], decoded[cIdx+1:]
	}
	ob := map[string]interface{}{"type": "shadowsocks", "server": host, "server_port": port, "method": method, "password": password}
	if plugin != "" {
		ob["plugin"] = plugin
	}
	if pluginOpts != "" {
		ob["plugin_opts"] = pluginOpts
	}
	return ob, name, nil
}

func parseNaive(uri string) (map[string]interface{}, string, error) {
	var content string
	quic := false
	switch {
	case strings.HasPrefix(uri, "naive+https://"):
		content = uri[len("naive+https://"):]
	case strings.HasPrefix(uri, "naive+quic://"):
		content = uri[len("naive+quic://"):]
		quic = true
	default:
		return nil, "", fmt.Errorf("invalid naive uri")
	}
	mainPart, name := splitName(content, "NaiveProxy")
	authority, _, _ := strings.Cut(mainPart, "?")
	var username, password string
	hasCreds := false
	hostPort := authority
	if atIdx := strings.LastIndexByte(authority, '@'); atIdx != -1 {
		userinfo := authority[:atIdx]
		hostPort = authority[atIdx+1:]
		hasCreds = true
		if cIdx := strings.IndexByte(userinfo, ':'); cIdx == -1 {
			username, password = pathUnescapeOr(userinfo), ""
		} else {
			username, password = pathUnescapeOr(userinfo[:cIdx]), pathUnescapeOr(userinfo[cIdx+1:])
		}
	}
	host, port, ok := splitHostPort(hostPort)
	if !ok {
		return nil, "", fmt.Errorf("naive: invalid host:port")
	}
	ob := map[string]interface{}{
		"type": "naive", "server": host, "server_port": port,
		"tls": map[string]interface{}{"enabled": true, "server_name": host},
	}
	if hasCreds {
		ob["username"] = username
		ob["password"] = password
	}
	if quic {
		ob["quic"] = true
	}
	return ob, name, nil
}

func orElse(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func pathUnescapeOr(s string) string {
	if dec, err := url.PathUnescape(s); err == nil {
		return dec
	}
	return s
}

// decodeSubscription normalizes a subscription body into trimmed non-empty link
// lines. If the raw body already contains "://" it is a plain link list;
// otherwise base64 decode is attempted (std/url, padded/unpadded) and used only
// if the decoded text contains "://".
func decodeSubscription(body []byte) []string {
	text := string(body)
	if !strings.Contains(text, "://") {
		trimmed := strings.TrimSpace(text)
		for _, dec := range tryBase64Variants(trimmed) {
			if strings.Contains(dec, "://") {
				text = dec
				break
			}
		}
	}
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, l := range raw {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func tryBase64Variants(s string) []string {
	var out []string
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			out = append(out, string(b))
		}
	}
	return out
}

// ParseLinks dispatches each line to the matching protocol parser; unknown
// schemes or parse failures increment skipped.
func ParseLinks(lines []string) (outbounds []ParsedNode, skipped int) {
	for _, line := range lines {
		var (
			ob   map[string]interface{}
			name string
			err  error
		)
		switch {
		case strings.HasPrefix(line, "vless://"):
			ob, name, err = parseVless(line)
		case strings.HasPrefix(line, "trojan://"):
			ob, name, err = parseTrojan(line)
		case strings.HasPrefix(line, "hy2://"), strings.HasPrefix(line, "hysteria2://"):
			ob, name, err = parseHysteria2(line)
		case strings.HasPrefix(line, "ss://"):
			ob, name, err = parseShadowsocks(line)
		case strings.HasPrefix(line, "naive+https://"), strings.HasPrefix(line, "naive+quic://"):
			ob, name, err = parseNaive(line)
		default:
			skipped++
			continue
		}
		if err != nil || ob == nil {
			skipped++
			continue
		}
		outbounds = append(outbounds, ParsedNode{Outbound: ob, Name: name})
	}
	return outbounds, skipped
}

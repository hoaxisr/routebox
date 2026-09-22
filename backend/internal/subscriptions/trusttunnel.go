package subscriptions

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// TrustTunnel share formats (ported from awg-manager internal/singbox/vlink):
// deep link `tt://?<base64url TLV>` and connect URL `http(s)://<any host>/…?d=<TLV>`.
// TLV: tag and length are RFC 9000 varints, unknown tags are skipped
// (DEEP_LINK.md, format version 1).
const ttMaxVersion = 1

const (
	ttTagVersion     = 0x00
	ttTagHostname    = 0x01
	ttTagAddresses   = 0x02
	ttTagCustomSNI   = 0x03
	ttTagUsername    = 0x05
	ttTagPassword    = 0x06
	ttTagSkipVerify  = 0x07
	ttTagCertificate = 0x08
	ttTagAntiDPI     = 0x0A
	ttTagName        = 0x0C
	// 0x04 has_ipv6, 0x09 upstream_protocol, 0x0B client_random_prefix,
	// 0x0D dns_upstreams: not read — the fork's outbound is H2 (quic: false).
)

type ttEndpoint struct {
	Hostname         string
	Addresses        []string
	CustomSNI        string
	Username         string
	Password         string
	SkipVerification bool
	Certificate      string // PEM chain or ""
	AntiDPI          bool
	Name             string
}

// isTrustTunnelLink reports whether line is a tt:// deep link or a connect URL.
func isTrustTunnelLink(line string) bool {
	if strings.HasPrefix(line, "tt://") {
		return true
	}
	u, err := url.Parse(line)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Query().Get("d") != ""
}

// parseTrustTunnel returns one outbound per address; the label is the connect
// URL's name=, else the TLV name, else the hostname. Tags get a -N suffix when
// the endpoint carries several addresses.
func parseTrustTunnel(line string) ([]ParsedNode, error) {
	var payload, label string
	if strings.HasPrefix(line, "tt://") {
		payload = strings.TrimPrefix(line[len("tt://"):], "?")
	} else {
		u, err := url.Parse(line)
		if err != nil {
			return nil, fmt.Errorf("trusttunnel: connect url: %w", err)
		}
		payload, label = u.Query().Get("d"), u.Query().Get("name")
	}
	ep, err := decodeTTPayload(payload)
	if err != nil {
		return nil, err
	}
	if label == "" {
		label = ep.Name
	}
	if label == "" {
		label = ep.Hostname
	}
	sni := ep.CustomSNI
	if sni == "" {
		sni = ep.Hostname
	}
	multi := len(ep.Addresses) > 1
	nodes := make([]ParsedNode, 0, len(ep.Addresses))
	for i, addr := range ep.Addresses {
		host, port, ok := splitHostPort(addr)
		if !ok {
			return nil, fmt.Errorf("trusttunnel: address %q: invalid host:port", addr)
		}
		tag := label
		if multi {
			tag = fmt.Sprintf("%s-%d", label, i+1)
		}
		tls := map[string]interface{}{"enabled": true, "server_name": sni}
		if ep.SkipVerification {
			tls["insecure"] = true
		}
		if ep.Certificate != "" {
			tls["certificate"] = strings.Split(strings.TrimRight(ep.Certificate, "\n"), "\n")
		}
		if ep.AntiDPI {
			tls["fragment"] = true // anti_dpi → sing-box's own TLS record fragmentation
		}
		nodes = append(nodes, ParsedNode{Name: tag, Outbound: map[string]interface{}{
			"type": "trusttunnel", "server": host, "server_port": port,
			"username": ep.Username, "password": ep.Password,
			"tls": tls,
		}})
	}
	return nodes, nil
}

func decodeTTPayload(b64 string) (ttEndpoint, error) {
	if b64 == "" {
		return ttEndpoint{}, errors.New("trusttunnel: empty payload")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(b64, "="))
	if err != nil {
		return ttEndpoint{}, fmt.Errorf("trusttunnel: base64url: %w", err)
	}
	return parseTTTLV(raw)
}

// readVarint reads an RFC 9000 varint: value, bytes consumed.
func readVarint(b []byte) (uint64, int, error) {
	if len(b) == 0 {
		return 0, 0, errors.New("trusttunnel: truncated TLV")
	}
	n := 1 << (b[0] >> 6)
	if len(b) < n {
		return 0, 0, errors.New("trusttunnel: truncated varint")
	}
	v := uint64(b[0] & 0x3f)
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[i])
	}
	return v, n, nil
}

func parseTTTLV(data []byte) (ttEndpoint, error) {
	var ep ttEndpoint
	for len(data) > 0 {
		tag, n, err := readVarint(data)
		if err != nil {
			return ttEndpoint{}, err
		}
		data = data[n:]
		length, n, err := readVarint(data)
		if err != nil {
			return ttEndpoint{}, err
		}
		data = data[n:]
		if uint64(len(data)) < length {
			return ttEndpoint{}, errors.New("trusttunnel: truncated TLV value")
		}
		val := data[:length]
		data = data[length:]
		switch tag {
		case ttTagVersion:
			v, _, err := readVarint(val)
			if err != nil {
				return ttEndpoint{}, err
			}
			if v > ttMaxVersion {
				return ttEndpoint{}, fmt.Errorf("trusttunnel: deep link version %d not supported (max %d)", v, ttMaxVersion)
			}
		case ttTagHostname:
			ep.Hostname = string(val)
		case ttTagAddresses:
			ep.Addresses = append(ep.Addresses, string(val))
		case ttTagCustomSNI:
			ep.CustomSNI = string(val)
		case ttTagUsername:
			ep.Username = string(val)
		case ttTagPassword:
			ep.Password = string(val)
		case ttTagSkipVerify:
			ep.SkipVerification = len(val) == 1 && val[0] == 1
		case ttTagCertificate:
			pemChain, err := derChainToPEM(val)
			if err != nil {
				return ttEndpoint{}, err
			}
			ep.Certificate = pemChain
		case ttTagAntiDPI:
			ep.AntiDPI = len(val) == 1 && val[0] == 1
		case ttTagName:
			ep.Name = string(val)
		}
	}
	switch {
	case ep.Hostname == "":
		return ttEndpoint{}, errors.New("trusttunnel: no hostname")
	case len(ep.Addresses) == 0:
		return ttEndpoint{}, errors.New("trusttunnel: no addresses")
	case ep.Username == "" || ep.Password == "":
		return ttEndpoint{}, errors.New("trusttunnel: no username/password")
	}
	return ep, nil
}

func derChainToPEM(der []byte) (string, error) {
	certs, err := x509.ParseCertificates(der)
	if err != nil {
		return "", fmt.Errorf("trusttunnel: certificate: %w", err)
	}
	var buf bytes.Buffer
	for _, c := range certs {
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	}
	return buf.String(), nil
}

package mtproto

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/9seconds/mtg/v2/essentials"
	"github.com/9seconds/mtg/v2/mtglib"
)

// errSocksRefused is the sentinel behind every non-zero SOCKS5 reply code, so a
// caller can tell "the outbound turned us away" apart from "the proxy is not
// there at all" without matching on message text.
var errSocksRefused = errors.New("the socks5 proxy refused the connection")

const (
	// socksHandshakeTimeout bounds the greeting + CONNECT exchange. The proxy is
	// on loopback and the outbound behind it is what actually takes time, so
	// this is generous: it exists to stop a wedged sing-box from pinning a
	// worker forever, not to police latency.
	socksHandshakeTimeout = 30 * time.Second

	// socksProxyDialTimeout bounds reaching the proxy itself. Loopback either
	// answers at once or is not listening.
	socksProxyDialTimeout = 5 * time.Second

	// DefaultSocksPort is the loopback port the managed inbound binds when the
	// setting is unset. Nothing off-host can reach it, so the conventional SOCKS
	// port carries none of the risk it would on a public interface.
	DefaultSocksPort = 1080
)

// SocksPortOrDefault resolves the configured loopback port, so the settings
// layer and the config sync cannot disagree about which port is meant.
func SocksPortOrDefault(port int) int {
	if port <= 0 {
		return DefaultSocksPort
	}

	return port
}

// SocksProxyAddr returns the proxy address for a chosen outbound, or "" when
// Telegram is reached directly. It is the single place that decides that an
// empty outbound means "no proxy" — everything else just passes the result on.
func SocksProxyAddr(outbound string, port int) string {
	if outbound == "" {
		return ""
	}

	return net.JoinHostPort("127.0.0.1", strconv.Itoa(SocksPortOrDefault(port)))
}

// socksNetwork is the mtglib.Network that reaches Telegram through a SOCKS5
// proxy — in RouteBox that proxy is the managed `mtproto-socks` inbound, and
// what is on the far side of it is whichever outbound or endpoint the operator
// picked on the Telegram page.
//
// It fails closed. When the proxy is unreachable or the outbound turns the
// connection away, Dial returns the error; there is deliberately no direct
// retry, because a silent fallback would reach Telegram from the server's own
// IP — undoing the one thing choosing an outbound is for.
//
// Domain fronting is the exception, and not by our choice: mtglib fronts using
// NativeDialer, documented as the dialer that skips the proxy. That is the right
// behaviour anyway — a fronted request should look like an ordinary visitor of
// the masking site, not like one arriving out of a VPN exit.
type socksNetwork struct {
	proxy       string
	dialer      *net.Dialer
	idleTimeout time.Duration
}

// newSocksNetwork builds a Network that CONNECTs through proxyAddr ("host:port").
func newSocksNetwork(proxyAddr string, idleTimeout time.Duration) mtglib.Network {
	return &socksNetwork{
		proxy: proxyAddr,
		dialer: &net.Dialer{
			Timeout:         socksProxyDialTimeout,
			KeepAliveConfig: net.KeepAliveConfig{Enable: true},
		},
		idleTimeout: idleTimeout,
	}
}

func (n *socksNetwork) Dial(network, address string) (essentials.Conn, error) {
	return n.DialContext(context.Background(), network, address)
}

func (n *socksNetwork) DialContext(ctx context.Context, network, address string) (essentials.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		// mtglib only ever asks for TCP; anything else is a caller bug, and
		// SOCKS5 CONNECT could not carry it regardless.
		return nil, fmt.Errorf("unsupported network %s", network)
	}

	// "tcp" rather than `network`: the address family that matters is the one
	// the OUTBOUND uses to reach Telegram, and the hop to the proxy is loopback.
	// Forcing tcp6 here would fail on a host with IPv6 disabled while the
	// outbound behind the proxy is perfectly capable of reaching a v6 DC.
	conn, err := n.dialer.DialContext(ctx, "tcp", n.proxy)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the socks5 proxy at %s: %w", n.proxy, err)
	}

	// The handshake gets its own deadline so a proxy that accepts and then says
	// nothing cannot hold the connection open indefinitely.
	deadline := time.Now().Add(socksHandshakeTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close() //nolint: errcheck
		return nil, fmt.Errorf("cannot set the handshake deadline: %w", err)
	}

	if err := socksConnect(conn, address); err != nil {
		conn.Close() //nolint: errcheck
		return nil, fmt.Errorf("cannot connect to %s via %s: %w", address, n.proxy, err)
	}

	// Clear it again: from here the conn is a plain relay stream and mtglib sets
	// whatever deadlines it wants. Leaving the handshake deadline armed would
	// kill every long-lived Telegram connection 30 seconds in.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close() //nolint: errcheck
		return nil, fmt.Errorf("cannot clear the handshake deadline: %w", err)
	}

	return essentials.WrapNetConn(conn), nil
}

// MakeHTTPClient mirrors the direct network's: mtglib uses it to fetch
// Telegram's public config, which should travel the same path as the traffic.
func (n *socksNetwork) MakeHTTPClient(
	dialFunc func(context.Context, string, string) (essentials.Conn, error),
) *http.Client {
	if dialFunc == nil {
		dialFunc = n.DialContext
	}

	return &http.Client{
		Timeout: socksHandshakeTimeout,
		Transport: &http.Transport{
			IdleConnTimeout: n.idleTimeout,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialFunc(ctx, network, address)
			},
		},
	}
}

// NativeDialer returns a dialer that does NOT go through the proxy — mtglib
// defines it that way, and domain fronting is its only caller. See the type
// comment for why that is the behaviour we want.
func (n *socksNetwork) NativeDialer() *net.Dialer {
	return &net.Dialer{Timeout: socksProxyDialTimeout}
}

// socksConnect performs the SOCKS5 no-auth greeting and a CONNECT to address
// (RFC 1928) on an already-established conn, leaving it positioned at the start
// of the tunnelled stream.
//
// A hostname is forwarded AS A NAME rather than resolved here, so sing-box
// resolves it through the chosen outbound. Resolving locally would both leak the
// lookup and pick an address from the wrong vantage point.
func socksConnect(conn net.Conn, address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("cannot parse the target address %s: %w", address, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("invalid target port %q", portStr)
	}

	// Greeting: SOCKS5, one method offered, "no authentication required". The
	// managed inbound has no users, which is exactly this method.
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return fmt.Errorf("cannot send the greeting: %w", err)
	}

	var greeting [2]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return fmt.Errorf("cannot read the greeting reply: %w", err)
	}

	if greeting[0] != 5 {
		return fmt.Errorf("the proxy answered SOCKS version %d, not 5", greeting[0])
	}

	if greeting[1] != 0 {
		// 0xFF is "no acceptable methods", which here means the proxy wants
		// credentials. Worth naming: it points straight at the inbound having
		// grown a users list.
		return fmt.Errorf("%w: it requires authentication (method %#x)", errSocksRefused, greeting[1])
	}

	req, err := appendSocksTarget([]byte{5, 1, 0}, host)
	if err != nil {
		return err
	}

	req = binary.BigEndian.AppendUint16(req, uint16(port))

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("cannot send the connect request: %w", err)
	}

	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return fmt.Errorf("cannot read the connect reply: %w", err)
	}

	if head[0] != 5 {
		return fmt.Errorf("the proxy answered SOCKS version %d, not 5", head[0])
	}

	if head[1] != 0 {
		return socksReplyError(head[1])
	}

	// The bound address is of no use to us, but it is on the wire ahead of the
	// tunnelled bytes and has to come off before the caller reads anything.
	return drainSocksAddr(conn, head[3])
}

// appendSocksTarget encodes host as a SOCKS5 address: a literal IP keeps its
// family, anything else goes as a domain name for the far side to resolve.
func appendSocksTarget(req []byte, host string) ([]byte, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return append(append(req, 1), v4...), nil
		}

		return append(append(req, 4), ip.To16()...), nil
	}

	if len(host) == 0 || len(host) > 255 {
		// The length prefix is one byte, so a longer name cannot be encoded at
		// all — better a named error than a silently truncated destination.
		return nil, fmt.Errorf("hostname %q does not fit a SOCKS5 request (%d bytes)", host, len(host))
	}

	req = append(req, 3, byte(len(host)))

	return append(req, host...), nil
}

// drainSocksAddr consumes the BND.ADDR/BND.PORT that follow a reply header.
func drainSocksAddr(conn net.Conn, atyp byte) error {
	var n int

	switch atyp {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		var length [1]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return fmt.Errorf("cannot read the bound address length: %w", err)
		}

		n = int(length[0])
	default:
		return fmt.Errorf("the proxy replied with unknown address type %d", atyp)
	}

	if _, err := io.ReadFull(conn, make([]byte, n+2)); err != nil {
		return fmt.Errorf("cannot read the bound address: %w", err)
	}

	return nil
}

// socksReplyError turns a SOCKS5 reply code into something an operator reading
// the Telegram log view can act on. "unsuccessful request: 3" tells them
// nothing; "network unreachable" points at the outbound they just picked.
func socksReplyError(code byte) error {
	var reason string

	switch code {
	case 1:
		reason = "general SOCKS server failure"
	case 2:
		reason = "not allowed by ruleset"
	case 3:
		reason = "network unreachable"
	case 4:
		reason = "host unreachable"
	case 5:
		reason = "connection refused"
	case 6:
		reason = "TTL expired"
	case 7:
		reason = "command not supported"
	case 8:
		reason = "address type not supported"
	default:
		reason = fmt.Sprintf("unknown reply code %d", code)
	}

	return fmt.Errorf("%w: %s", errSocksRefused, reason)
}

package mtproto

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// socksRequest is what the fake server saw, so a test can assert that the
// address really was forwarded rather than dialed behind the proxy's back.
type socksRequest struct {
	atyp byte
	host string
	port int
}

// fakeSocks serves ONE SOCKS5 CONNECT, replies with `rep`, and — on success —
// echoes whatever the client sends afterwards. It is deliberately strict about
// the greeting: a proxy that accepts a malformed one would hide exactly the
// bugs this exercises.
func fakeSocks(t *testing.T, rep byte) (addr string, seen <-chan socksRequest) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { lis.Close() })

	got := make(chan socksRequest, 1)

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var greeting [3]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			return
		}
		if greeting[0] != 5 || greeting[1] != 1 || greeting[2] != 0 {
			return // not "SOCKS5, one method, no auth" — say nothing, fail the test by timeout
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			return
		}

		var head [4]byte
		if _, err := io.ReadFull(conn, head[:]); err != nil {
			return
		}

		req := socksRequest{atyp: head[3]}

		switch head[3] {
		case 1:
			var v4 [4]byte
			io.ReadFull(conn, v4[:]) //nolint: errcheck
			req.host = net.IP(v4[:]).String()
		case 3:
			var n [1]byte
			io.ReadFull(conn, n[:]) //nolint: errcheck
			name := make([]byte, n[0])
			io.ReadFull(conn, name) //nolint: errcheck
			req.host = string(name)
		case 4:
			var v6 [16]byte
			io.ReadFull(conn, v6[:]) //nolint: errcheck
			req.host = net.IP(v6[:]).String()
		}

		var port [2]byte
		io.ReadFull(conn, port[:]) //nolint: errcheck
		req.port = int(binary.BigEndian.Uint16(port[:]))

		got <- req

		// A bound address of 0.0.0.0:0 is what a proxy that does not care to
		// report one sends; the client must drain it either way.
		conn.Write([]byte{5, rep, 0, 1, 0, 0, 0, 0, 0, 0}) //nolint: errcheck

		if rep != 0 {
			return
		}

		io.Copy(conn, conn) //nolint: errcheck
	}()

	return lis.Addr().String(), got
}

func TestSocksNetworkDialsThroughTheProxy(t *testing.T) {
	proxy, seen := fakeSocks(t, 0)

	ntw := newSocksNetwork(proxy, time.Minute)

	conn, err := ntw.DialContext(context.Background(), "tcp4", "149.154.167.51:443")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()

	select {
	case req := <-seen:
		if req.atyp != 1 {
			t.Errorf("atyp = %d, want 1 (IPv4)", req.atyp)
		}
		if req.host != "149.154.167.51" || req.port != 443 {
			t.Errorf("target = %s:%d, want 149.154.167.51:443", req.host, req.port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the proxy never saw a CONNECT")
	}

	// The conn must survive the handshake as a usable stream: mtglib relays
	// bytes over it immediately after Dial returns.
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 5)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint: errcheck
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(buf) != "hello" {
		t.Errorf("echo = %q, want %q", buf, "hello")
	}
}

// The handshake must not clear the deadline mtglib is about to rely on... but it
// also must not leave its own handshake deadline armed, or a long-lived relay
// would die at an arbitrary moment. A fresh conn has no deadline; so must this.
func TestSocksNetworkClearsItsHandshakeDeadline(t *testing.T) {
	proxy, _ := fakeSocks(t, 0)

	conn, err := newSocksNetwork(proxy, time.Minute).Dial("tcp", "1.2.3.4:80")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Nothing is written for longer than the handshake timeout would have
	// allowed; a leftover deadline shows up here as a timeout error.
	time.Sleep(50 * time.Millisecond)
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatalf("write after the handshake: %v", err)
	}
}

func TestSocksNetworkForwardsDomains(t *testing.T) {
	proxy, seen := fakeSocks(t, 0)

	conn, err := newSocksNetwork(proxy, time.Minute).Dial("tcp", "core.telegram.org:443")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	req := <-seen
	// Passing the NAME (not a locally-resolved IP) is what lets sing-box resolve
	// it through the chosen outbound instead of leaking the lookup locally.
	if req.atyp != 3 || req.host != "core.telegram.org" {
		t.Errorf("target = atyp %d %q, want atyp 3 \"core.telegram.org\"", req.atyp, req.host)
	}
}

func TestSocksNetworkForwardsIPv6(t *testing.T) {
	proxy, seen := fakeSocks(t, 0)

	conn, err := newSocksNetwork(proxy, time.Minute).Dial("tcp6", "[2001:67c:4e8:f004::9]:443")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	req := <-seen
	if req.atyp != 4 || req.host != "2001:67c:4e8:f004::9" {
		t.Errorf("target = atyp %d %q, want atyp 4 IPv6", req.atyp, req.host)
	}
}

// A refusal from the outbound must name what happened. This is the error the
// operator reads in the Telegram log view when the outbound they picked is
// broken, and "unsuccessful request: 5" is not something anyone can act on.
func TestSocksNetworkReportsRefusal(t *testing.T) {
	proxy, _ := fakeSocks(t, 5) // 5 = connection refused

	_, err := newSocksNetwork(proxy, time.Minute).Dial("tcp", "1.2.3.4:80")
	if err == nil {
		t.Fatal("want an error, got a connection")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want it to mention a refused connection", err)
	}
}

// Fail closed: with the proxy down, dialing must fail rather than quietly fall
// back to a direct connection that would reach Telegram from the server's own IP.
func TestSocksNetworkFailsClosedWhenTheProxyIsDown(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := lis.Addr().String()
	lis.Close() // nothing is listening there now

	_, err = newSocksNetwork(dead, time.Minute).Dial("tcp", "1.2.3.4:80")
	if err == nil {
		t.Fatal("want an error, got a connection")
	}
	if !strings.Contains(err.Error(), dead) {
		t.Errorf("error = %q, want it to name the unreachable proxy %s", err, dead)
	}
}

func TestSocksNetworkRejectsNonTCP(t *testing.T) {
	proxy, _ := fakeSocks(t, 0)

	if _, err := newSocksNetwork(proxy, time.Minute).Dial("udp", "1.2.3.4:80"); err == nil {
		t.Fatal("want an error for a udp dial, got a connection")
	}
}

// Domain fronting deliberately bypasses the proxy (mtglib calls NativeDialer for
// it), so the fronted request looks like an ordinary visitor of the masking site
// rather than one arriving out of the operator's VPN exit.
func TestSocksNetworkNativeDialerIsDirect(t *testing.T) {
	proxy, seen := fakeSocks(t, 0)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	go func() {
		if c, err := lis.Accept(); err == nil {
			c.Close()
		}
	}()

	conn, err := newSocksNetwork(proxy, time.Minute).NativeDialer().Dial("tcp", lis.Addr().String())
	if err != nil {
		t.Fatalf("NativeDialer: %v", err)
	}
	conn.Close()

	select {
	case req := <-seen:
		t.Fatalf("NativeDialer went through the proxy (saw %s:%d)", req.host, req.port)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSocksReplyError(t *testing.T) {
	for code, want := range map[byte]string{
		1: "general SOCKS server failure",
		2: "not allowed by ruleset",
		3: "network unreachable",
		4: "host unreachable",
		5: "connection refused",
		6: "TTL expired",
		7: "command not supported",
		8: "address type not supported",
	} {
		err := socksReplyError(code)
		if err == nil {
			t.Fatalf("code %d: want an error", code)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("code %d: error = %q, want it to contain %q", code, err, want)
		}
		if !errors.Is(err, errSocksRefused) {
			t.Errorf("code %d: want it to wrap errSocksRefused", code)
		}
	}

	// An unknown code still has to produce something, not a nil error the
	// caller would read as success.
	if err := socksReplyError(200); err == nil {
		t.Error("unknown code: want an error")
	}
}

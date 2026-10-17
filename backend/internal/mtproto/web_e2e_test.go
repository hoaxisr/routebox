package mtproto

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/9seconds/mtg/v2/essentials"
	"github.com/9seconds/mtg/v2/mtglib"
	"github.com/9seconds/mtg/v2/mtglib/obfuscation"
	"github.com/9seconds/mtg/v2/mtglib/web"

	"routebox/backend/internal/webfront"
)

// Spec §7.4: bytes from a WEB proxy client go through the web front, the
// manager's web.Handler and the proxy to a Telegram datacenter and back, and
// the traffic is billed to the client whose secret matched. The datacenter is
// an in-process echo server the manager's network seam dials instead of
// Telegram; everything between it and the client is the production wiring.
//
// The client side is a trimmed copy of the fork's test-only bridge client
// (mtglib/web/testclient_test.go): one obfuscated2 stream over the /up and
// /down carrier, honouring the server's send window.

const (
	e2eWait     = 5 * time.Second
	e2eHostname = "proxy.example.com"
	e2eXFF      = "203.0.113.9"
	e2eDC       = 2
	octetStream = "application/octet-stream"
	clientUpCap = 1 << 20
	clientRetry = 20 * time.Millisecond
)

func TestWebProxyEndToEndBillsTheMatchedClient(t *testing.T) {
	m := testManager(t)
	m.networkOverride = &fakeDCNetwork{addr: startFakeDC(t)}

	// Two clients, so a match on the wrong index would bill the wrong name.
	for _, c := range []Client{
		{Name: "alice", Secret: secretA, Enabled: true},
		{Name: "bob", Secret: secretB, Enabled: true},
	} {
		if err := m.Store().Put(c); err != nil {
			t.Fatal(err)
		}
	}

	front := webfront.New(http.NotFoundHandler())
	if err := front.Start(freeAddr(t)); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = front.Stop() })
	m.SetWebFront(front)

	cfg := testConfig(t)
	cfg.Web = &WebConfig{Hostname: e2eHostname, Public: http.NotFoundHandler()}

	if err := m.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The client talks to the front, never to the handler directly: that hop
	// is the one this test exists to cover.
	base := "http://" + front.Listen()

	bob := newBridgeClient(t, base, secretOf(secretB, cfg.MaskingDomain))
	bob.connect()
	t.Cleanup(func() { bob.close() })

	data := make([]byte, 200<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	s := bob.openStream(1)
	if got := roundTrip(t, s, data); !bytes.Equal(got, data) {
		t.Fatal("the echo through the WEB proxy differs from what was sent")
	}

	// Traffic events arrive on the relay's goroutines, a moment after the
	// bytes they count.
	var totals map[string]Totals

	deadline := time.Now().Add(e2eWait)
	for {
		totals = m.Events().Cumulative()
		if b := totals["bob"]; b.Upload >= int64(len(data)) && b.Download >= int64(len(data)) {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("bob's traffic never reached %d bytes each way: %+v", len(data), totals)
		}

		time.Sleep(5 * time.Millisecond)
	}

	if a, ok := totals["alice"]; ok {
		t.Errorf("alice was billed %+v for bob's stream: the matched index points at the wrong client", a)
	}

	drained := m.Events().DrainTotals()
	if b := drained["bob"]; b.Upload < int64(len(data)) || b.Download < int64(len(data)) {
		t.Errorf("DrainTotals = %+v, want bob's stream in both directions", drained)
	}

	conns := m.Events().Connections()
	if len(conns) != 1 || conns[0].Client != "bob" || conns[0].ClientIP != e2eXFF {
		t.Errorf("Connections = %+v, want one stream of bob's from %s", conns, e2eXFF)
	}
}

// fakeDCNetwork ignores the requested address and connects to an in-process
// fake datacenter.
type fakeDCNetwork struct{ addr string }

func (f *fakeDCNetwork) Dial(_, _ string) (essentials.Conn, error) {
	c, err := net.Dial("tcp", f.addr)
	if err != nil {
		return nil, err
	}

	return essentials.WrapNetConn(c), nil
}

func (f *fakeDCNetwork) DialContext(_ context.Context, network, address string) (essentials.Conn, error) {
	return f.Dial(network, address)
}

func (f *fakeDCNetwork) MakeHTTPClient(func(context.Context, string, string) (essentials.Conn, error)) *http.Client {
	return &http.Client{}
}

func (f *fakeDCNetwork) NativeDialer() *net.Dialer { return &net.Dialer{} }

// startFakeDC accepts direct-DC obfuscated2 connections and echoes them.
func startFakeDC(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}

			go func() {
				defer c.Close()

				_, conn, err := obfuscation.Obfuscator{}.ReadHandshake(essentials.WrapNetConn(c))
				if err != nil {
					return
				}

				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	return l.Addr().String()
}

// roundTrip writes data to a stream and reads the same amount back.
func roundTrip(t *testing.T, s *clientStream, data []byte) []byte {
	t.Helper()

	type result struct {
		buf []byte
		err error
	}

	done := make(chan result, 1)

	go func() {
		buf := make([]byte, len(data))
		_, err := io.ReadFull(s, buf)
		done <- result{buf, err}
	}()

	// A copy: the obfuscated2 conn encrypts in place.
	if _, err := s.conn.Write(append([]byte(nil), data...)); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("read: %v", r.err)
		}

		return r.buf
	case <-time.After(10 * time.Second):
		t.Fatalf("no echo of %d bytes", len(data))

		return nil
	}
}

var bootstrapRe = regexp.MustCompile(`const BOOTSTRAP = "([A-Za-z0-9_-]{43})";`)

// bridgeClient is the bridge page in Go: it creates a session and carries
// streams over /up and /down.
type bridgeClient struct {
	t       *testing.T
	baseURL string
	secret  mtglib.Secret
	http    *http.Client
	token   string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// mu guards everything below; cond is broadcast on every change.
	mu       sync.Mutex
	cond     *sync.Cond
	outbox   []byte
	streams  map[uint32]*clientStream
	stopping bool

	closeOnce sync.Once
}

// clientStream is one obfuscated2 stream whose bytes travel as DATA frames.
type clientStream struct {
	c    *bridgeClient
	id   uint32
	conn essentials.Conn

	// guarded by c.mu
	credit       int64
	inbound      []byte
	remoteClosed bool
}

func newBridgeClient(t *testing.T, baseURL string, secret mtglib.Secret) *bridgeClient {
	ctx, cancel := context.WithCancel(context.Background())
	c := &bridgeClient{
		t:       t,
		baseURL: baseURL,
		secret:  secret,
		http:    &http.Client{Transport: &http.Transport{}},
		ctx:     ctx,
		cancel:  cancel,
		streams: map[uint32]*clientStream{},
	}
	c.cond = sync.NewCond(&c.mu)

	return c
}

func (c *bridgeClient) request(method, path string, body []byte) *http.Request {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(c.ctx, method, c.baseURL+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}

	// What a reverse proxy in front of the web front would send.
	req.Host = e2eHostname
	req.Header.Set("X-Forwarded-For", e2eXFF)

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return req
}

func (c *bridgeClient) do(req *http.Request) (*http.Response, []byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	return resp, body, err
}

// connect fetches the bridge page, creates a session and starts the carrier.
func (c *bridgeClient) connect() {
	c.t.Helper()

	resp, page, err := c.do(c.request(http.MethodGet, "/?bridge="+web.Capability(c.secret, e2eHostname), nil))
	if err != nil || resp.StatusCode != http.StatusOK {
		c.t.Fatalf("bridge page: %v (status %v)", err, resp)
	}

	m := bootstrapRe.FindSubmatch(page)
	if m == nil {
		c.t.Fatal("no bootstrap token in the bridge page: the front did not reach the WEB handler")
	}

	c.token = string(m[1])

	req := c.request(http.MethodPost, "/api/v1/session", web.EncodeFrame(web.FrameHello, 0, []byte{1}))
	req.Header.Set("Content-Type", octetStream)

	resp, body, err := c.do(req)
	if err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, web.EncodeFrame(web.FrameWelcome, 0, nil)) {
		c.t.Fatalf("session: %v (status %v, body %x)", err, resp, body)
	}

	c.token = resp.Header.Get("X-Session-Token")
	if c.token == "" {
		c.t.Fatal("no session token")
	}

	c.wg.Add(2)

	go c.upLoop()
	go c.downLoop()
}

// openStream opens a stream and performs the client obfuscated2 handshake.
func (c *bridgeClient) openStream(id uint32) *clientStream {
	c.t.Helper()

	s := &clientStream{c: c, id: id, credit: web.InitialStreamWindow}
	s.conn = essentials.WrapNetConn(rawStreamConn{s})

	c.mu.Lock()
	c.streams[id] = s
	c.queueLocked(web.FrameOpen, id, nil)
	c.mu.Unlock()

	conn, err := obfuscation.Obfuscator{Secret: c.secret.Key[:]}.SendHandshake(s.conn, e2eDC)
	if err != nil {
		c.t.Fatalf("obfuscated2 handshake: %v", err)
	}

	s.conn = conn

	return s
}

func (c *bridgeClient) queueLocked(t web.FrameType, id uint32, payload []byte) {
	c.outbox = web.AppendFrame(c.outbox, t, id, payload)
	c.cond.Broadcast()
}

// close deletes the session and stops the loops.
func (c *bridgeClient) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.stopping = true
		c.cond.Broadcast()
		c.mu.Unlock()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.baseURL+"/api/v1/session", nil)
		if err == nil {
			req.Host = e2eHostname
			req.Header.Set("X-Forwarded-For", e2eXFF)
			req.Header.Set("Authorization", "Bearer "+c.token)
			_, _, _ = c.do(req)
		}

		c.cancel()
		c.wg.Wait()
		c.http.CloseIdleConnections()
	})
}

// fail reports a carrier failure from a background loop and stops the client.
func (c *bridgeClient) fail(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.stopping {
		c.t.Errorf(format, args...)
		c.stopping = true
		c.cond.Broadcast()
	}
}

func (c *bridgeClient) isStopping() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.stopping
}

// takeBatchLocked cuts whole frames, up to clientUpCap bytes and
// MaxBatchFrames frames, off the outbox.
func (c *bridgeClient) takeBatchLocked() []byte {
	size, frames := 0, 0

	for size < len(c.outbox) && frames < web.MaxBatchFrames {
		frameSize := web.FrameHeaderSize + int(binary.BigEndian.Uint32(c.outbox[size+4:size+web.FrameHeaderSize]))
		if size > 0 && size+frameSize > clientUpCap {
			break
		}

		size += frameSize
		frames++
	}

	batch := c.outbox[:size:size]
	c.outbox = c.outbox[size:]

	return batch
}

func (c *bridgeClient) upLoop() {
	defer c.wg.Done()

	for seq := uint64(1); ; seq++ {
		c.mu.Lock()

		for len(c.outbox) == 0 && !c.stopping {
			c.cond.Wait()
		}

		if c.stopping {
			c.mu.Unlock()

			return
		}

		batch := c.takeBatchLocked()
		c.mu.Unlock()

		if !c.sendUp(seq, batch) {
			return
		}
	}
}

// sendUp posts one batch, retrying it on 503.
func (c *bridgeClient) sendUp(seq uint64, batch []byte) bool {
	for {
		req := c.request(http.MethodPost, "/api/v1/up", batch)
		req.Header.Set("Content-Type", octetStream)
		req.Header.Set("X-Up-Seq", strconv.FormatUint(seq, 10))

		resp, _, err := c.do(req)

		switch {
		case c.isStopping():
			return false
		case err != nil:
			c.fail("up %d: %v", seq, err)

			return false
		case resp.StatusCode == http.StatusServiceUnavailable:
			time.Sleep(clientRetry)
		case resp.StatusCode != http.StatusNoContent:
			c.fail("up %d: status %d", seq, resp.StatusCode)

			return false
		default:
			return true
		}
	}
}

func (c *bridgeClient) downLoop() {
	defer c.wg.Done()

	cursor := "0"

	for {
		req := c.request(http.MethodPost, "/api/v1/down", nil)
		req.Header.Set("X-Down-Cursor", cursor)

		resp, body, err := c.do(req)

		switch {
		case c.isStopping():
			return
		case err != nil:
			c.fail("down %s: %v", cursor, err)

			return
		case resp.StatusCode == http.StatusNoContent:
		case resp.StatusCode != http.StatusOK:
			c.fail("down %s: status %d", cursor, resp.StatusCode)

			return
		default:
			frames, err := web.ParseFrames(body, 0)
			if err != nil {
				c.fail("down %s: %v", cursor, err)

				return
			}

			c.dispatch(frames)
		}

		cursor = resp.Header.Get("X-Down-Cursor")
	}
}

func (c *bridgeClient) dispatch(frames []web.Frame) {
	c.mu.Lock()
	defer c.mu.Unlock()

	defer c.cond.Broadcast()

	for _, f := range frames {
		s := c.streams[f.StreamID]

		switch f.Type { //nolint: exhaustive
		case web.FramePing:
			c.queueLocked(web.FramePong, 0, f.Payload)
		case web.FrameData:
			if s != nil {
				s.inbound = append(s.inbound, f.Payload...)
			}
		case web.FrameWindow:
			if n, err := web.WindowAmount(f.Payload); err == nil && s != nil {
				s.credit += int64(n)
			}
		case web.FrameClose:
			if s != nil {
				s.remoteClosed = true
			}
		}
	}
}

// Read hands out decrypted bytes from the stream.
func (s *clientStream) Read(p []byte) (int, error) { return s.conn.Read(p) }

func (s *clientStream) writeRaw(p []byte) (int, error) {
	c := s.c

	c.mu.Lock()
	defer c.mu.Unlock()

	written := 0

	for written < len(p) {
		for s.credit <= 0 && !s.remoteClosed && !c.stopping {
			c.cond.Wait()
		}

		if s.remoteClosed || c.stopping {
			return written, net.ErrClosed
		}

		n := min(int64(len(p)-written), web.DataChunk, s.credit)
		c.queueLocked(web.FrameData, s.id, p[written:written+int(n)])
		s.credit -= n
		written += int(n)
	}

	return written, nil
}

func (s *clientStream) readRaw(p []byte) (int, error) {
	c := s.c

	c.mu.Lock()
	defer c.mu.Unlock()

	for len(s.inbound) == 0 {
		switch {
		case s.remoteClosed:
			return 0, io.EOF
		case c.stopping:
			return 0, net.ErrClosed
		}

		c.cond.Wait()
	}

	n := copy(p, s.inbound)
	s.inbound = s.inbound[n:]

	if !s.remoteClosed {
		c.queueLocked(web.FrameWindow, s.id, web.WindowPayload(uint32(n))) //nolint: gosec
	}

	return n, nil
}

var errNoDeadlines = errors.New("deadlines are not supported")

// rawStreamConn is the net.Conn face of a clientStream's DATA plumbing.
type rawStreamConn struct{ s *clientStream }

func (r rawStreamConn) Read(p []byte) (int, error)  { return r.s.readRaw(p) }
func (r rawStreamConn) Write(p []byte) (int, error) { return r.s.writeRaw(p) }
func (rawStreamConn) Close() error                  { return nil }
func (rawStreamConn) LocalAddr() net.Addr           { return &net.TCPAddr{} }
func (rawStreamConn) RemoteAddr() net.Addr          { return &net.TCPAddr{} }
func (rawStreamConn) SetDeadline(time.Time) error   { return errNoDeadlines }
func (rawStreamConn) SetReadDeadline(time.Time) error {
	return errNoDeadlines
}
func (rawStreamConn) SetWriteDeadline(time.Time) error { return errNoDeadlines }

package subscriptions

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
)

// resetServer answers requests with body, but hard-RSTs the first n connections
// it accepts (SO_LINGER 0 → RST, not FIN — what a reset on the path looks like).
// conns counts every accepted connection.
func resetServer(t *testing.T, resetFirst int32, body string) (url string, conns *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	conns = &atomic.Int32{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			n := conns.Add(1)
			go func(c net.Conn, n int32) {
				defer c.Close()
				if n <= resetFirst {
					if tc, ok := c.(*net.TCPConn); ok {
						_ = tc.SetLinger(0) // close() now sends RST
					}
					return
				}
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(line) == "" {
						break
					}
				}
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
			}(c, n)
		}
	}()
	return "http://" + ln.Addr().String() + "/sub/tok", conns
}

// #77: a subscription refresh died on a single "connection reset by peer" —
// every other manual Refresh threw the raw Go error at the operator, and the
// 12-hour auto-refresh failed the same way. Go's transport retries a reset only
// on a POOLED connection (proved by TestFetchSurvivesResetOnPooledConn); on a
// freshly dialled one nothing retries, so the fetcher has to.
func TestFetchRetriesTransientReset(t *testing.T) {
	url, conns := resetServer(t, 1, "dmxlc3M6Ly8=\n")
	body, err := Fetch(url)
	if err != nil {
		t.Fatalf("a single reset must not fail the refresh: %v", err)
	}
	if string(body) != "dmxlc3M6Ly8=\n" {
		t.Fatalf("body = %q", body)
	}
	if got := conns.Load(); got != 2 {
		t.Fatalf("expected one retry (2 connections), got %d", got)
	}
}

// Two resets in a row are still survivable — the reported path dropped roughly
// every other connection, so one retry alone would have left a visible failure
// every fourth click.
func TestFetchRetriesTwoResetsInARow(t *testing.T) {
	url, conns := resetServer(t, 2, "dmxlc3M6Ly8=\n")
	if _, err := Fetch(url); err != nil {
		t.Fatalf("two resets must still refresh: %v", err)
	}
	if got := conns.Load(); got != 3 {
		t.Fatalf("expected 3 connections, got %d", got)
	}
}

// The retry is bounded: a server that resets everything still returns the error
// (with the original wording, so the panel keeps naming the real cause) instead
// of hanging or looping.
func TestFetchGivesUpOnPersistentReset(t *testing.T) {
	url, conns := resetServer(t, 100, "")
	if _, err := Fetch(url); err == nil {
		t.Fatal("a server that resets every attempt must surface an error")
	} else if !strings.Contains(err.Error(), "reset") && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("error should name the transport failure, got %v", err)
	}
	if got := conns.Load(); got != 3 {
		t.Fatalf("retries must be bounded to 3 attempts, got %d connections", got)
	}
}

// Go's own transport already covers the pooled-connection case: the second Fetch
// draws the dead connection out of the shared pool and is retried transparently.
// Documented as a test so a future "let's add our own pool handling" has the
// evidence in front of it.
func TestFetchSurvivesResetOnPooledConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	body := "dmxlc3M6Ly8=\n"
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				for req := 0; ; req++ {
					for {
						line, err := br.ReadString('\n')
						if err != nil {
							return
						}
						if strings.TrimSpace(line) == "" {
							break
						}
					}
					if req > 0 { // second request on this kept-alive connection
						if tc, ok := c.(*net.TCPConn); ok {
							_ = tc.SetLinger(0)
						}
						return
					}
					fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
				}
			}(c)
		}
	}()
	url := "http://" + ln.Addr().String() + "/sub/tok"
	for i := 0; i < 2; i++ {
		if _, err := Fetch(url); err != nil {
			t.Fatalf("fetch %d: %v", i+1, err)
		}
	}
}

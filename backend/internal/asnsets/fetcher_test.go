package asnsets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func ripe(t *testing.T, h http.HandlerFunc) *Fetcher {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	f := NewFetcher()
	f.BaseURL = srv.URL
	f.RetryDelay = 0
	return f
}

func TestPrefixesParsesAnnouncedPrefixes(t *testing.T) {
	f := ripe(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/announced-prefixes/data.json" ||
			r.URL.Query().Get("resource") != "AS13335" || r.URL.Query().Get("sourceapp") != "routebox" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":{"resource":"13335","prefixes":[
			{"prefix":"1.1.1.0/24","timelines":[]},{"prefix":"2606:4700::/32"},{"prefix":"garbage"}]}}`))
	})
	got, err := f.Prefixes(context.Background(), 13335)
	if err != nil || len(got) != 2 || got[0].String() != "1.1.1.0/24" || got[1].String() != "2606:4700::/32" {
		t.Fatalf("got %v err=%v", got, err)
	}
}

func TestPrefixesEmptyIsNotAnError(t *testing.T) {
	f := ripe(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","data":{"prefixes":[]}}`))
	})
	got, err := f.Prefixes(context.Background(), 4294967290)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v err=%v", got, err)
	}
}

func TestFetchErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"non-200": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
		"status error": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"error","messages":[["error","bad"]]}`))
		},
		"bad json": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`<html>`)) },
	} {
		var calls atomic.Int32
		f := ripe(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			h(w, r)
		})
		if _, err := f.Prefixes(context.Background(), 1); err == nil {
			t.Errorf("%s: want error", name)
		}
		// None of these is a transport error, so none earns a retry.
		if n := calls.Load(); n != 1 {
			t.Errorf("%s: %d requests, want exactly 1 (no retry)", name, n)
		}
	}
}

func TestHolder(t *testing.T) {
	f := ripe(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/as-overview/data.json") {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":{"holder":"CLOUDFLARENET - Cloudflare, Inc.","announced":true}}`))
	})
	got, err := f.Holder(context.Background(), 13335)
	if err != nil || got != "CLOUDFLARENET - Cloudflare, Inc." {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestTransportErrorIsRetried(t *testing.T) {
	calls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			hj, _ := w.(http.Hijacker)
			c, _, _ := hj.Hijack()
			c.Close() // connection reset → *url.Error
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":{"prefixes":[{"prefix":"1.1.1.0/24"}]}}`))
	}))
	defer srv.Close()
	f := NewFetcher()
	f.BaseURL, f.RetryDelay = srv.URL, 0
	got, err := f.Prefixes(context.Background(), 13335)
	if err != nil || len(got) != 1 || calls != 3 {
		t.Fatalf("got %v err=%v calls=%d", got, err, calls)
	}
}

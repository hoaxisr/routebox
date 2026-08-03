package subscriptions

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeMerger struct {
	groupTag, nodePrefix string
	nodes                []map[string]interface{}
	group                map[string]interface{}
	called, removed      int
}

func (f *fakeMerger) ReplaceSubscriptionOutbounds(groupTag, nodePrefix string, nodes []map[string]interface{}, group map[string]interface{}) error {
	f.called++
	f.groupTag, f.nodePrefix, f.nodes, f.group = groupTag, nodePrefix, nodes, group
	return nil
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestSanitize(t *testing.T) {
	cases := map[string]string{"Tokyo 01": "Tokyo 01", "hk/relay#2": "hkrelay2", "a·b": "ab", "  pad  ": "pad"}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRefreshSuccess(t *testing.T) {
	body := b64("ss://YWVzLTEyOC1nY206cGFzcw==@1.2.3.4:8388#Tokyo\nvless://uuid@5.6.7.8:443#Osaka")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "RouteBox" {
			t.Errorf("missing User-Agent: %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	fm := &fakeMerger{}
	n, skipped, err := Refresh(Subscription{ID: "home", Name: "Home", URL: srv.URL}, fm)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || skipped != 0 {
		t.Fatalf("nodeCount=%d skipped=%d", n, skipped)
	}
	if fm.called != 1 || fm.groupTag != "Home" || fm.nodePrefix != "Home · " {
		t.Fatalf("merge call wrong: %d %q %q", fm.called, fm.groupTag, fm.nodePrefix)
	}
	for _, node := range fm.nodes {
		if tag, _ := node["tag"].(string); !strings.HasPrefix(tag, "Home · ") {
			t.Fatalf("node tag not prefixed: %q", tag)
		}
	}
	if fm.group["type"] != "urltest" || fm.group["tag"] != "Home" ||
		fm.group["url"] != "https://www.gstatic.com/generate_204" || fm.group["interval"] != "3m" {
		t.Fatalf("group shape wrong: %+v", fm.group)
	}
	if obs, _ := fm.group["outbounds"].([]interface{}); len(obs) != 2 {
		t.Fatalf("group.outbounds = %v", fm.group["outbounds"])
	}
}

func TestRefreshCollisionSuffix(t *testing.T) {
	body := b64("ss://YWVzLTEyOC1nY206cGFzcw==@1.2.3.4:8388#Dup\nss://YWVzLTEyOC1nY206cGFzcw==@5.6.7.8:8388#Dup")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer srv.Close()
	fm := &fakeMerger{}
	if _, _, err := Refresh(Subscription{Name: "S", URL: srv.URL}, fm); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, node := range fm.nodes {
		tag := node["tag"].(string)
		if seen[tag] {
			t.Fatalf("duplicate tag %q", tag)
		}
		seen[tag] = true
	}
	if !seen["S · Dup"] || !seen["S · Dup-2"] {
		t.Fatalf("collision suffixing missing: %v", seen)
	}
}

func TestRefreshNoNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(b64("garbage-no-uri"))) }))
	defer srv.Close()
	fm := &fakeMerger{}
	_, _, err := Refresh(Subscription{Name: "S", URL: srv.URL}, fm)
	if err == nil {
		t.Fatal("expected error on zero usable nodes")
	}
	// The error must say WHY nothing was usable (issue #50: an unsupported
	// scheme and an empty body both surfaced as a bare "no usable nodes").
	if !strings.Contains(err.Error(), "no usable nodes") || !strings.Contains(err.Error(), "1 link") {
		t.Fatalf("error should count the skipped links: %v", err)
	}
	if fm.called != 0 {
		t.Fatal("merger must NOT be called with no nodes")
	}
}

func TestRefreshEmptyBody(t *testing.T) {
	// An inactive/empty panel user serves a 200 with an empty body — the error
	// must name that, not the misleading "no usable nodes".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("  \n\n")) }))
	defer srv.Close()
	fm := &fakeMerger{}
	_, _, err := Refresh(Subscription{Name: "S", URL: srv.URL}, fm)
	if err == nil {
		t.Fatal("expected error on empty body")
	}
	if !strings.Contains(err.Error(), "subscription is empty") {
		t.Fatalf("error = %v, want \"subscription is empty\"", err)
	}
	if fm.called != 0 {
		t.Fatal("merger must NOT be called on empty body")
	}
}

func TestRefreshNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	fm := &fakeMerger{}
	if _, _, err := Refresh(Subscription{Name: "S", URL: srv.URL}, fm); err == nil {
		t.Fatal("expected error on non-2xx")
	}
	if fm.called != 0 {
		t.Fatal("merger must NOT be called on fetch error")
	}
}

package asnsets

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "asn.toml")
	s := NewStore(p)
	e := Entry{Tag: "cf", ASNs: []uint32{13335}, Holders: map[string]string{"13335": "Cloudflare"}, IntervalHrs: 24, UpdatedAt: 100, PrefixCount: 7}
	if err := s.Put(e); err != nil {
		t.Fatal(err)
	}
	_ = s.Put(Entry{Tag: "a", ASNs: []uint32{1}, IntervalHrs: 6})
	s2 := NewStore(p)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	list := s2.List()
	if len(list) != 2 || list[0].Tag != "a" || list[1].Holders["13335"] != "Cloudflare" || list[1].PrefixCount != 7 {
		t.Fatalf("list = %+v", list)
	}
	if err := s2.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("a"); ok {
		t.Fatal("a still present")
	}
}

func TestStoreMissingFileIsEmpty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "none.toml"))
	if err := s.Load(); err != nil || len(s.List()) != 0 {
		t.Fatalf("err=%v list=%v", err, s.List())
	}
}

func TestWritePrefixFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "asn", "cf.json")
	if err := WritePrefixFile(p, []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24"), netip.MustParsePrefix("2606:4700::/32")}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
		Rules   []struct {
			IPCIDR []string `json:"ip_cidr"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != 2 || len(doc.Rules) != 1 || len(doc.Rules[0].IPCIDR) != 2 {
		t.Fatalf("doc=%+v err=%v raw=%s", doc, err, raw)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestWritePrefixFileEmptyMatchesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	if err := WritePrefixFile(p, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if strings.TrimSpace(string(raw)) != `{"rules":[],"version":2}` {
		t.Fatalf("got %s", raw)
	}
}

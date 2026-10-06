package asnsets

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/util"
)

func TestStoreRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "asn.toml")
	s := NewStore(p)
	e := Entry{Tag: "cf", ASNs: []uint32{13335}, Holders: map[string]string{"13335": "Cloudflare"}, IntervalHrs: 24, UpdatedAt: 100, PrefixCount: 7}
	if err := s.Put(e); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Entry{Tag: "a", ASNs: []uint32{1}, IntervalHrs: 6}); err != nil {
		t.Fatal(err)
	}
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

// readOnlyDir returns a directory RouteBox cannot write into. Mode bits mean
// nothing to root, so the test is skipped there rather than made to pass.
func readOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("root ignores mode bits")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	return dir
}

func TestStorePutReadOnlyDirRollsBack(t *testing.T) {
	dir := readOnlyDir(t)
	s := NewStore(filepath.Join(dir, "asn.toml"))
	err := s.Put(Entry{Tag: "x", ASNs: []uint32{1}, IntervalHrs: 24})
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Put err = %v, want ErrReadOnly", err)
	}
	if list := s.List(); len(list) != 0 {
		t.Fatalf("entry survived a failed write: %+v", list)
	}
	if _, ok := s.Get("x"); ok {
		t.Fatal("Get found the rolled-back entry")
	}
	if _, err := os.Stat(filepath.Join(dir, "asn.toml")); !os.IsNotExist(err) {
		t.Fatalf("file state: %v", err)
	}
}

func TestStoreDeleteReadOnlyDirRollsBack(t *testing.T) {
	dir := readOnlyDir(t)
	s := NewStore(filepath.Join(dir, "asn.toml"))
	// Seed memory without touching disk, the way a successful earlier Put
	// followed by a remount read-only would leave it.
	s.byTag["x"] = Entry{Tag: "x", ASNs: []uint32{1}}
	err := s.Delete("x")
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("Delete err = %v, want ErrReadOnly", err)
	}
	if _, ok := s.Get("x"); !ok {
		t.Fatal("entry vanished from memory although the write failed")
	}
}

func TestWritePrefixFileReadOnlyDir(t *testing.T) {
	dir := readOnlyDir(t)
	err := WritePrefixFile(filepath.Join(dir, "x.json"), []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24")})
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("leftovers in read-only dir: %v", entries)
	}
}

// A hand-edited or corrupt asn.toml must be refused as a whole: a partial or
// unsafe set of entries would be rewritten over the file on the first Put
// (metadata of every other set gone) or let PathFor escape asn/.
func TestStoreLoadRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"invalid TOML":  "[[sets]\ntag = \"cf\"\n",
		"unsafe tag":    "[[sets]]\ntag = \"../x\"\nasns = [1]\n",
		"empty tag":     "[[sets]]\ntag = \"\"\nasns = [1]\n",
		"duplicate tag": "[[sets]]\ntag = \"cf\"\nasns = [1]\n\n[[sets]]\ntag = \"cf\"\nasns = [2]\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "asn.toml")
			if err := os.WriteFile(p, []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
			s := NewStore(p)
			s.byTag["keep"] = Entry{Tag: "keep"} // whatever was there before Load
			err := s.Load()
			if err == nil {
				t.Fatalf("Load accepted %s: %v", name, s.List())
			}
			if !strings.Contains(err.Error(), p) {
				t.Errorf("error does not name the file: %v", err)
			}
			if _, ok := s.Get("keep"); !ok {
				t.Error("a failed Load replaced the in-memory entries")
			}
		})
	}
}

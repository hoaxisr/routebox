package asnsets

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"routebox/backend/internal/util"
)

type fakeSrc struct {
	prefixes map[uint32][]string
	err      map[uint32]error
	calls    int
}

func (f *fakeSrc) Prefixes(_ context.Context, asn uint32) ([]netip.Prefix, error) {
	f.calls++
	if e := f.err[asn]; e != nil {
		return nil, e
	}
	var out []netip.Prefix
	for _, s := range f.prefixes[asn] {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out, nil
}
func (f *fakeSrc) Holder(_ context.Context, asn uint32) (string, error) {
	if asn == 13335 {
		return "Cloudflare", nil
	}
	return "", errors.New("no holder")
}

func newTestManager(t *testing.T, src *fakeSrc) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	m := NewManager(NewStore(filepath.Join(dir, "asn.toml")), filepath.Join(dir, "asn"), src)
	m.now = func() int64 { return 1000 }
	return m, dir
}

var ctx = context.Background()

func TestCreateWritesFileAndEntry(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{13335: {"1.1.1.0/24", "1.0.0.0/24"}, 32934: {"31.13.24.0/21"}}}
	m, _ := newTestManager(t, src)
	e, err := m.Create(ctx, "cf", []uint32{13335, 32934, 13335}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.ASNs) != 2 || e.IntervalHrs != 24 || e.PrefixCount != 3 || e.UpdatedAt != 1000 || e.Holders["13335"] != "Cloudflare" || e.Holders["32934"] != "" {
		t.Fatalf("entry = %+v", e)
	}
	if _, err := os.Stat(e.Path); err != nil || e.Path != m.PathFor("cf") {
		t.Fatalf("file %s: %v", e.Path, err)
	}
}

func TestCreateRejectsAsnWithoutPrefixes(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{13335: {"1.1.1.0/24"}}}
	m, _ := newTestManager(t, src)
	_, err := m.Create(ctx, "x", []uint32{13335, 64512}, 24)
	var fe *FetchError
	if !errors.As(err, &fe) || fe.ASN != 64512 {
		t.Fatalf("err = %v, want FetchError for AS64512", err)
	}
	if _, ok := m.Get("x"); ok {
		t.Fatal("entry created despite error")
	}
	if _, err := os.Stat(m.PathFor("x")); !os.IsNotExist(err) {
		t.Fatal("file written despite error")
	}
}

func TestCreateRejectsUnsafeTag(t *testing.T) {
	m, _ := newTestManager(t, &fakeSrc{})
	for _, tag := range []string{"../x", "a/b", "", ".."} {
		if _, err := m.Create(ctx, tag, []uint32{1}, 24); !errors.Is(err, ErrInvalid) {
			t.Errorf("tag %q: err = %v, want ErrInvalid", tag, err)
		}
	}
	if _, err := m.Create(ctx, "ok", []uint32{1}, 5); !errors.Is(err, ErrInvalid) {
		t.Errorf("interval 5 accepted")
	}
}

func TestRefreshFailureKeepsFile(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{13335: {"1.1.1.0/24"}}}
	m, _ := newTestManager(t, src)
	e, _ := m.Create(ctx, "cf", []uint32{13335}, 24)
	before, _ := os.ReadFile(e.Path)

	src.err = map[uint32]error{13335: errors.New("RIPEstat down")}
	if _, err := m.Refresh(ctx, "cf"); err == nil {
		t.Fatal("want error")
	}
	after, _ := os.ReadFile(e.Path)
	got, _ := m.Get("cf")
	if string(before) != string(after) || got.LastError == "" || got.UpdatedAt != 1000 {
		t.Fatalf("file changed or entry wrong: %+v", got)
	}

	src.err, src.prefixes = nil, map[uint32][]string{13335: {}}
	if _, err := m.Refresh(ctx, "cf"); err == nil {
		t.Fatal("empty union must be an error")
	}
	after, _ = os.ReadFile(e.Path)
	if string(before) != string(after) {
		t.Fatal("empty refresh overwrote the file")
	}

	src.prefixes = map[uint32][]string{13335: {"1.1.1.0/24", "104.16.0.0/13"}}
	m.now = func() int64 { return 2000 }
	got, err := m.Refresh(ctx, "cf")
	if err != nil || got.LastError != "" || got.PrefixCount != 2 || got.UpdatedAt != 2000 || got.Holders["13335"] != "Cloudflare" {
		t.Fatalf("recovery: %+v err=%v", got, err)
	}
}

func TestUpdateChangesAsnsKeepsPath(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{13335: {"1.1.1.0/24"}, 32934: {"31.13.24.0/21"}}}
	m, _ := newTestManager(t, src)
	e1, _ := m.Create(ctx, "cf", []uint32{13335}, 24)
	e2, err := m.Update(ctx, "cf", []uint32{32934}, 6)
	if err != nil || e2.Path != e1.Path || e2.IntervalHrs != 6 || e2.ASNs[0] != 32934 {
		t.Fatalf("e2=%+v err=%v", e2, err)
	}
	if _, err := m.Update(ctx, "nope", []uint32{1}, 24); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestPruneKeepsTagsInEitherConfig(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	a, _ := m.Create(ctx, "a", []uint32{1}, 24)
	b, _ := m.Create(ctx, "b", []uint32{1}, 24)
	m.now = func() int64 { return 1000 + pruneGrace } // both past the grace period
	// "a" was deleted from the draft but the active config still has it.
	m.Prune(func(tag string) bool { return tag == "a" })
	if _, err := os.Stat(a.Path); err != nil {
		t.Fatal("a's file removed while still referenced")
	}
	if _, ok := m.Get("b"); ok {
		t.Fatal("b not pruned")
	}
	if _, err := os.Stat(b.Path); !os.IsNotExist(err) {
		t.Fatal("b's file not removed")
	}
}

func TestRestoreMissingRefetches(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	e, _ := m.Create(ctx, "a", []uint32{1}, 24)
	_ = os.Remove(e.Path)
	m.RestoreMissing(ctx)
	raw, err := os.ReadFile(e.Path)
	if err != nil || !strings.Contains(string(raw), "1.0.0.0/24") {
		t.Fatalf("file not restored: %s %v", raw, err)
	}
}

func TestRestoreMissingWritesPlaceholderWhenOffline(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	e, _ := m.Create(ctx, "a", []uint32{1}, 24)
	_ = os.Remove(e.Path)
	src.err = map[uint32]error{1: errors.New("offline")}
	m.RestoreMissing(ctx)
	raw, err := os.ReadFile(e.Path)
	if err != nil || strings.TrimSpace(string(raw)) != `{"rules":[],"version":2}` {
		t.Fatalf("placeholder missing: %s %v", raw, err)
	}
	got, _ := m.Get("a")
	if got.LastError == "" {
		t.Fatal("LastError not set")
	}
	if got.UpdatedAt != 0 {
		t.Fatalf("UpdatedAt = %d, want 0 so the next tick retries instead of waiting a full interval", got.UpdatedAt)
	}
}

func TestRestoreMissingPlaceholderIsDueOnNextTick(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	const boot = int64(1_700_000_000) // a real epoch: UpdatedAt=0 is "ages ago"
	m.now = func() int64 { return boot }
	e, _ := m.Create(ctx, "a", []uint32{1}, 168) // a week: far from due
	_ = os.Remove(e.Path)
	src.err = map[uint32]error{1: errors.New("offline")}
	m.RestoreMissing(ctx)

	src.err = nil
	m.now = func() int64 { return boot + 3600 } // one tick later
	m.RefreshDue(ctx)
	raw, _ := os.ReadFile(e.Path)
	got, _ := m.Get("a")
	if !strings.Contains(string(raw), "1.0.0.0/24") || got.LastError != "" || got.UpdatedAt != boot+3600 {
		t.Fatalf("placeholder not replaced on the next tick: %s %+v", raw, got)
	}
}

func TestPruneSparesFreshSets(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	m.now = func() int64 { return 1000 }
	old, _ := m.Create(ctx, "old", []uint32{1}, 24) // UpdatedAt 1000
	m.now = func() int64 { return 1600 }
	fresh, _ := m.Create(ctx, "fresh", []uint32{1}, 24) // UpdatedAt 1600
	// now = 1700: "fresh" is 100 s old (the handler may still be adding it to
	// the draft), "old" is 700 s old. Neither is in any config.
	m.now = func() int64 { return 1700 }
	m.Prune(func(string) bool { return false })
	if _, ok := m.Get("fresh"); !ok {
		t.Fatal("fresh set pruned inside the grace period")
	}
	if _, err := os.Stat(fresh.Path); err != nil {
		t.Fatal("fresh set's file removed inside the grace period")
	}
	if _, ok := m.Get("old"); ok {
		t.Fatal("old orphan not pruned")
	}
	if _, err := os.Stat(old.Path); !os.IsNotExist(err) {
		t.Fatal("old orphan's file not removed")
	}
}

// blockingSrc hangs until ctx is done, like RIPEstat with a black-holed route.
type blockingSrc struct{}

func (blockingSrc) Prefixes(ctx context.Context, _ uint32) ([]netip.Prefix, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingSrc) Holder(ctx context.Context, _ uint32) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestRefreshDueReturnsWhenCtxExpires(t *testing.T) {
	// Seed a due entry through a working source, then swap in one that hangs.
	live := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, live)
	e, _ := m.Create(ctx, "a", []uint32{1}, 6)
	m.src = blockingSrc{}
	m.now = func() int64 { return 1000 + 7*3600 }

	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { m.RefreshDue(short); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RefreshDue did not return after its ctx expired; the lock would be held for as long as the source hangs")
	}
	raw, _ := os.ReadFile(e.Path)
	got, _ := m.Get("a")
	if !strings.Contains(string(raw), "1.0.0.0/24") || got.LastError == "" || got.UpdatedAt != 1000 {
		t.Fatalf("timed-out refresh must keep the file and record the error: %s %+v", raw, got)
	}
	// The lock is free again: a plain call must not block either.
	if _, err := m.Refresh(short, "a"); err == nil {
		t.Fatal("Refresh with an expired ctx must fail, not hang or succeed")
	}
}

func TestRefreshDueOnlyRefreshesDue(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	_, _ = m.Create(ctx, "a", []uint32{1}, 6)  // due after 6h
	_, _ = m.Create(ctx, "b", []uint32{1}, 24) // due after 24h
	src.calls = 0
	m.now = func() int64 { return 1000 + 7*3600 }
	m.RefreshDue(ctx)
	if src.calls != 1 {
		t.Fatalf("calls = %d, want 1 (only a)", src.calls)
	}
}

// Fetcher is the production PrefixSource; keep the two in step.
var _ PrefixSource = (*Fetcher)(nil)

func TestCreateExistingTagIsInvalid(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	if _, err := m.Create(ctx, "a", []uint32{1}, 24); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, "a", []uint32{1}, 24); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want ErrInvalid (already exists)", err)
	}
	if _, err := m.Refresh(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Refresh err = %v, want ErrNotFound", err)
	}
}

func TestCreateStorePutFailureRemovesFile(t *testing.T) {
	// asn.toml lives in a read-only dir, the prefix dir is writable: the prefix
	// file is written first and must be removed when the entry cannot be saved.
	ro := readOnlyDir(t)
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m := NewManager(NewStore(filepath.Join(ro, "asn.toml")), filepath.Join(t.TempDir(), "asn"), src)
	_, err := m.Create(ctx, "a", []uint32{1}, 24)
	if !errors.Is(err, util.ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	if _, err := os.Stat(m.PathFor("a")); !os.IsNotExist(err) {
		t.Fatalf("prefix file left behind: %v", err)
	}
	if _, ok := m.Get("a"); ok {
		t.Fatal("entry present despite failed Put")
	}
}

func TestRefreshDueLogsOncePerState(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	_, _ = m.Create(ctx, "a", []uint32{1}, 6)
	now := int64(1000)
	m.now = func() int64 { return now }
	tick := func() { now += 7 * 3600; m.RefreshDue(ctx) } // always past the 6 h interval

	tick() // success right away is not news
	if buf.Len() != 0 {
		t.Fatalf("unexpected log on plain success: %q", buf.String())
	}

	src.err = map[uint32]error{1: errors.New("down")}
	tick()
	tick() // same error again: silent
	if n := strings.Count(buf.String(), "refresh failed"); n != 1 {
		t.Fatalf("failed logged %d times, want 1: %q", n, buf.String())
	}
	src.err = map[uint32]error{1: errors.New("still down, differently")}
	tick()
	if n := strings.Count(buf.String(), "refresh failed"); n != 2 {
		t.Fatalf("new error text not logged: %q", buf.String())
	}

	src.err = nil
	tick()
	tick() // ok stays ok: silent
	if n := strings.Count(buf.String(), "refresh ok again"); n != 1 {
		t.Fatalf("recovery logged %d times, want 1: %q", n, buf.String())
	}
}

func TestDeleteRemovesEntryAndFile(t *testing.T) {
	src := &fakeSrc{prefixes: map[uint32][]string{1: {"1.0.0.0/24"}}}
	m, _ := newTestManager(t, src)
	e, _ := m.Create(ctx, "a", []uint32{1}, 24)
	if err := m.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("a"); ok {
		t.Fatal("entry survived Delete")
	}
	if _, err := os.Stat(e.Path); !os.IsNotExist(err) {
		t.Fatalf("file survived Delete: %v", err)
	}
	if err := m.Delete("a"); err != nil {
		t.Fatalf("second Delete must be a no-op, got %v", err)
	}
}

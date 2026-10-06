package awg

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"routebox/backend/internal/quota"
)

// seedUsagePeer stores one peer and returns it. Kernel tests key the store by a
// short literal (the store takes any string); only the peer ops validate keys.
func seedUsagePeer(t *testing.T, m *Manager, p Peer) {
	t.Helper()
	if err := m.store.Put(p); err != nil {
		t.Fatalf("seed peer: %v", err)
	}
}

// The tick folds the LIVE counters' deltas into the stored cumulative ones, and
// an interface restart (counters back to near-zero) must not roll the stored
// totals back — that is the whole reason the numbers are kept beside the peer
// instead of read off `awg show` (spec Q7).
func TestSweepAccumulatesUsageDeltas(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32", UsedRx: 4000, UsedTx: 3000})

	// The FIRST snapshot of the process only primes the reference. The tunnel
	// outlives RouteBox — a kernel Rehydrate adopts a live awg-rb0, a singbox
	// sync is change-gated — so its counters already include everything the
	// stored totals hold, and counting them again would double a peer's usage on
	// every restart.
	f.outputs["awg show awg-rb0 transfer"] = "P\t100\t50\n"
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 4000 || got.UsedTx != 3000 {
		t.Fatalf("the priming tick must not count anything: used = %d/%d, want 4000/3000", got.UsedRx, got.UsedTx)
	}

	// Steady state: only the delta.
	f.outputs["awg show awg-rb0 transfer"] = "P\t250\t80\n"
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 4150 || got.UsedTx != 3030 {
		t.Fatalf("second tick: used = %d/%d, want 4150/3030", got.UsedRx, got.UsedTx)
	}

	// Interface restarted: the live counter is lower than the last snapshot, so
	// the current value IS the delta — the stored totals never go down.
	f.outputs["awg show awg-rb0 transfer"] = "P\t30\t10\n"
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 4180 || got.UsedTx != 3040 {
		t.Fatalf("after a counter reset: used = %d/%d, want 4180/3040", got.UsedRx, got.UsedTx)
	}
}

// A peer that drops out of the live snapshot (suspended, or the interface
// forgot it) contributes no delta, and its last value is forgotten so that
// re-admission counts from its fresh counter instead of a stale high-water mark.
//
// The return value is deliberately ABOVE the forgotten reference: at 7 against a
// remembered 500 the reset rule (cur < last -> cur) would give the same answer,
// and the test would pass with the forgetting removed. At 600 the two rules
// disagree — forgotten means +600, remembered means +100 — so only one of them
// can make it green.
func TestSweepForgetsPeerMissingFromSnapshot(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32"})

	f.outputs["awg show awg-rb0 transfer"] = "P\t500\t500\n"
	m.SweepExpired(ctx) // primes the reference at 500/500

	f.outputs["awg show awg-rb0 transfer"] = "" // gone from the interface
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 0 || got.UsedTx != 0 {
		t.Fatalf("an absent peer must contribute no delta: used = %d/%d", got.UsedRx, got.UsedTx)
	}

	// Back on a fresh counter that has already passed the forgotten reference.
	f.outputs["awg show awg-rb0 transfer"] = "P\t600\t600\n"
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 600 || got.UsedTx != 600 {
		t.Fatalf("a returning peer must count its whole fresh counter: used = %d/%d, want 600/600 (100/100 means the stale reference was kept)", got.UsedRx, got.UsedTx)
	}
}

// peers.toml holds client private keys on a router's flash. A tick that moved no
// bytes must not rewrite it — the sweep runs every 30s forever.
func TestSweepDoesNotRewritePeersTomlWithoutChanges(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32"})

	path := m.store.GetPath()
	old := time.Unix(1000000, 0)
	mtime := func(t *testing.T) time.Time {
		t.Helper()
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return st.ModTime()
	}

	// The priming tick counts nothing, so it must write nothing either.
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	f.outputs["awg show awg-rb0 transfer"] = "P\t100\t50\n"
	m.SweepExpired(ctx)
	if !mtime(t).Equal(old) {
		t.Fatalf("peers.toml was rewritten on the priming tick (mtime %v)", mtime(t))
	}

	f.outputs["awg show awg-rb0 transfer"] = "P\t250\t80\n"
	m.SweepExpired(ctx) // this one DOES change the counters
	if mtime(t).Equal(old) {
		t.Fatal("setup: a tick that moves the counters must write peers.toml")
	}

	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	m.SweepExpired(ctx) // same snapshot: nothing to add
	if !mtime(t).Equal(old) {
		t.Fatalf("peers.toml was rewritten on a tick with no traffic (mtime %v)", mtime(t))
	}
}

// Enforcement must survive a peers.toml that cannot be written: the counters are
// what suspends a peer, and a read-only install would otherwise hand every client
// an unlimited allowance. The numbers stay in memory and the disk catches up on
// the next successful write; the failure is logged once, not every 30 seconds.
func TestSweepKeepsCountersWhenPeersTomlCannotBeWritten(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32", QuotaBytes: 100})
	good := m.store.GetPath()
	breakStore(t, m)

	f.outputs["awg show awg-rb0 transfer"] = "P\t0\t0\n"
	m.SweepExpired(ctx) // prime
	f.outputs["awg show awg-rb0 transfer"] = "P\t60\t50\n"
	m.SweepExpired(ctx)
	f.outputs["awg show awg-rb0 transfer"] = "P\t70\t60\n"
	m.SweepExpired(ctx)

	got, _ := m.store.Get("P")
	if got.UsedRx != 70 || got.UsedTx != 60 {
		t.Fatalf("counters must advance in memory despite the failed writes: used = %d/%d, want 70/60", got.UsedRx, got.UsedTx)
	}
	if !got.Suspended(2000) {
		t.Fatal("a peer over its quota must be out of service even when nothing can be persisted")
	}
	if n := strings.Count(buf.String(), "kept in memory only"); n != 1 {
		t.Fatalf("the write failure must be reported once across the ticks, got %d:\n%s", n, buf.String())
	}

	// Recovery is worth exactly one line too: the operator has to learn that the
	// numbers are on disk again.
	m.store.path = good
	f.outputs["awg show awg-rb0 transfer"] = "P\t90\t80\n"
	m.SweepExpired(ctx)
	if n := strings.Count(buf.String(), "writable again"); n != 1 {
		t.Fatalf("recovery must be reported once, got %d:\n%s", n, buf.String())
	}
	if n := strings.Count(buf.String(), "kept in memory only"); n != 1 {
		t.Fatalf("recovery must not re-report the failure, got %d:\n%s", n, buf.String())
	}
	if got, _ := m.store.Get("P"); got.UsedRx != 90 || got.UsedTx != 80 {
		t.Fatalf("the recovered write must carry the whole running total: used = %d/%d, want 90/80", got.UsedRx, got.UsedTx)
	}
}

// The suspension pass reads the counters this same tick folded in, so a peer
// that crosses its limit comes off the interface immediately, not one tick later.
func TestSweepSuspendsPeerCrossingQuotaSameTick(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32", QuotaBytes: 100})
	m.appendPeerToConf(PeerLine{Name: "x", PublicKey: "P", PSK: "p", AllowedIP: "10.10.0.2/32"})
	f.outputs["awg show awg-rb0"] = "peer: P\n"
	f.outputs["awg show awg-rb0 transfer"] = "P\t0\t0\n"
	m.SweepExpired(ctx) // prime, nothing spent yet
	if _, ok := m.store.Get("P"); !ok || f.sawContains("awg set awg-rb0 peer P remove") {
		t.Fatalf("setup: nothing may be suspended before any traffic; calls=%v", f.calls)
	}
	f.outputs["awg show awg-rb0 transfer"] = "P\t60\t50\n" // 110 >= 100

	m.SweepExpired(ctx)

	if got, _ := m.store.Get("P"); got.Suspension(2000) != quota.ReasonQuota {
		t.Fatalf("peer must be over quota after the tick: %+v", got)
	}
	if !f.sawContains("awg set awg-rb0 peer P remove") {
		t.Fatalf("expected the live remove in the same tick; calls=%v", f.calls)
	}
	if data, _ := os.ReadFile(m.confPath); strings.Contains(string(data), "PublicKey = P") {
		t.Fatalf("quota-exhausted peer still in conf:\n%s", data)
	}
}

// Q19: raising the limit puts the peer back on the interface at save time — the
// operator must not have to wait for a tick that would not admit it anyway.
func TestSetPeerLimitsRaisingQuotaAdmitsImmediately(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 1000 }
	seedUsagePeer(t, m, Peer{
		PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob",
		QuotaBytes: 100, UsedRx: 200,
	})

	if err := m.SetPeerLimits(ctx, validPub, nil, i64(1000)); err != nil {
		t.Fatalf("SetPeerLimits: %v", err)
	}
	got, _ := m.store.Get(validPub)
	if got.QuotaBytes != 1000 || got.ExpiresAt != 0 {
		t.Fatalf("the new limit must be persisted, the untouched date left alone: %+v", got)
	}
	if !f.sawContains("awg set awg-rb0 peer " + validPub) {
		t.Fatalf("expected an immediate re-admit; calls=%v", f.calls)
	}
	data, _ := os.ReadFile(m.confPath)
	if n := strings.Count(string(data), "PublicKey = "+validPub); n != 1 {
		t.Fatalf("expected exactly one conf block, got %d:\n%s", n, data)
	}
}

// Q20: lowering the limit below what is already spent is allowed and takes the
// peer off the interface at save time.
func TestSetPeerLimitsLoweringQuotaSuspendsImmediately(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 1000 }
	seedUsagePeer(t, m, Peer{
		PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob",
		UsedRx: 500, UsedTx: 500,
	})
	m.appendPeerToConf(PeerLine{Name: "bob", PublicKey: validPub, PSK: "psk", AllowedIP: "10.10.0.2/32"})

	if err := m.SetPeerLimits(ctx, validPub, nil, i64(100)); err != nil {
		t.Fatalf("SetPeerLimits: %v", err)
	}
	if !f.sawContains("awg set awg-rb0 peer " + validPub + " remove") {
		t.Fatalf("expected an immediate suspend; calls=%v", f.calls)
	}
	if data, _ := os.ReadFile(m.confPath); strings.Contains(string(data), "PublicKey = "+validPub) {
		t.Fatalf("peer must be out of the conf:\n%s", data)
	}
}

// A nil argument keeps the stored value, and the halves are resolved INSIDE the
// lock — that is the whole reason the handler passes its body pointers through
// instead of reading the peer itself: an expiry-only save must not write back a
// quota it read before the other operator's quota-only save landed.
func TestSetPeerLimitsNilKeepsTheStoredValue(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t, newFakeRunner())
	seedConf(t, m)
	m.store.now = func() int64 { return 1000 }
	seedUsagePeer(t, m, Peer{
		PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob",
		ExpiresAt: 500, QuotaBytes: 4096, UsedRx: 10,
	})

	// The expiry row: date only.
	if err := m.SetPeerLimits(ctx, validPub, i64(5000), nil); err != nil {
		t.Fatalf("SetPeerLimits: %v", err)
	}
	got, _ := m.store.Get(validPub)
	if got.ExpiresAt != 5000 || got.QuotaBytes != 4096 || got.UsedRx != 10 {
		t.Fatalf("an expiry-only save must move only the date: %+v", got)
	}

	// The quota row: limit only.
	if err := m.SetPeerLimits(ctx, validPub, nil, i64(8192)); err != nil {
		t.Fatalf("SetPeerLimits: %v", err)
	}
	got, _ = m.store.Get(validPub)
	if got.QuotaBytes != 8192 || got.ExpiresAt != 5000 || got.UsedRx != 10 {
		t.Fatalf("a quota-only save must move only the limit: %+v", got)
	}

	// Both nil: a no-op that still cannot lose anything.
	if err := m.SetPeerLimits(ctx, validPub, nil, nil); err != nil {
		t.Fatalf("SetPeerLimits: %v", err)
	}
	if got, _ = m.store.Get(validPub); got.ExpiresAt != 5000 || got.QuotaBytes != 8192 {
		t.Fatalf("two nils must change nothing: %+v", got)
	}
}

// Q9/Q19: resetting the counter zeroes it, stamps the reset moment, and returns
// the peer to service in the same call.
func TestResetPeerUsageZeroesAndAdmits(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 4242 }
	seedUsagePeer(t, m, Peer{
		PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob",
		QuotaBytes: 100, UsedRx: 90, UsedTx: 90,
	})

	if err := m.ResetPeerUsage(ctx, validPub); err != nil {
		t.Fatalf("ResetPeerUsage: %v", err)
	}
	got, _ := m.store.Get(validPub)
	if got.UsedRx != 0 || got.UsedTx != 0 {
		t.Fatalf("counters not zeroed: %+v", got)
	}
	if got.UsedResetAt != 4242 {
		t.Fatalf("UsedResetAt = %d, want the reset moment 4242", got.UsedResetAt)
	}
	if got.QuotaBytes != 100 {
		t.Fatalf("reset must not clear the limit: %+v", got)
	}
	if got.Suspended(4242) {
		t.Fatal("the peer must be back in service after a reset")
	}
	if !f.sawContains("awg set awg-rb0 peer " + validPub) {
		t.Fatalf("expected an immediate re-admit; calls=%v", f.calls)
	}
}

func TestResetPeerUsageUnknownPeer(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	seedConf(t, m)
	if err := m.ResetPeerUsage(context.Background(), otherValidPub); err != ErrPeerNotFound {
		t.Fatalf("want ErrPeerNotFound, got %v", err)
	}
}

// The roster carries the STORED cumulative numbers, not the live ones: the panel
// draws a quota bar, and a bar that resets whenever the interface restarts would
// tell the operator the peer got its allowance back.
func TestListPeersReportsStoredUsageAndQuota(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{
		PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32", Name: "bob",
		QuotaBytes: 1000, UsedRx: 700, UsedTx: 400, UsedResetAt: 123,
	})
	f.outputs["awg show awg-rb0 transfer"] = "P\t5\t5\n" // live counters differ

	peers := m.ListPeers(ctx)
	if len(peers) != 1 {
		t.Fatalf("want 1 peer, got %d", len(peers))
	}
	got := peers[0]
	if got.Rx != 700 || got.Tx != 400 {
		t.Fatalf("rx/tx must be the stored cumulative counters, got %d/%d", got.Rx, got.Tx)
	}
	if got.QuotaBytes != 1000 || got.UsedResetAt != 123 {
		t.Fatalf("quota fields missing from the summary: %+v", got)
	}
	if got.SuspendReason != quota.ReasonQuota {
		t.Fatalf("SuspendReason = %q, want %q", got.SuspendReason, quota.ReasonQuota)
	}
}

func TestListPeersActivePeerHasNoSuspendReason(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	seedConf(t, m)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", Address: "10.10.0.2/32", Name: "bob"})

	peers := m.ListPeers(context.Background())
	if len(peers) != 1 || peers[0].SuspendReason != quota.ReasonNone {
		t.Fatalf("an active peer must report no reason: %+v", peers)
	}
}

// The sing-box backend has no awg-rb0 to query: the same accounting runs off the
// fork's per-peer stats route (spec Q11), with no fallback to traffic history.
func TestSingboxSweepAccountsPeerStats(t *testing.T) {
	ctx := context.Background()
	m, _, _ := newSingboxMgr(t)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32"})

	stat := PeerStat{RxBytes: 100, TxBytes: 40}
	m.SetPeerStats(func() (map[string]PeerStat, error) {
		return map[string]PeerStat{"P": stat}, nil
	})

	// Same rule as on kernel: a sing-box endpoint survives a panel restart (the
	// sync is change-gated), so the first snapshot is a reference, not a bill.
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 0 || got.UsedTx != 0 {
		t.Fatalf("the priming tick must not count anything: used = %d/%d, want 0/0", got.UsedRx, got.UsedTx)
	}

	stat = PeerStat{RxBytes: 250, TxBytes: 140}
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 150 || got.UsedTx != 100 {
		t.Fatalf("second tick: used = %d/%d, want 150/100", got.UsedRx, got.UsedTx)
	}

	stat = PeerStat{RxBytes: 30, TxBytes: 90} // endpoint restarted: both counters reset
	m.SweepExpired(ctx)
	if got, _ := m.store.Get("P"); got.UsedRx != 180 || got.UsedTx != 190 {
		t.Fatalf("after a counter reset: used = %d/%d, want 180/190", got.UsedRx, got.UsedTx)
	}
}

// A peer that used up its allowance drops out of the rendered endpoint on the
// tick that notices, exactly as an expired one does.
func TestSingboxSweepSuspendsQuotaExhaustedPeer(t *testing.T) {
	ctx := context.Background()
	m, fs, _ := newSingboxMgr(t)
	if _, err := m.AddPeer(ctx, "bob"); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	pub := fs.lastSpec.Peers[0].PublicKey
	p, _ := m.store.Get(pub)
	p.QuotaBytes = 100
	if err := m.store.Put(p); err != nil {
		t.Fatal(err)
	}
	stat := PeerStat{}
	m.SetPeerStats(func() (map[string]PeerStat, error) {
		return map[string]PeerStat{pub: stat}, nil
	})

	m.SweepExpired(ctx) // prime
	stat = PeerStat{RxBytes: 80, TxBytes: 80}
	m.SweepExpired(ctx)

	if got, _ := m.store.Get(pub); got.UsedRx != 80 || got.UsedTx != 80 {
		t.Fatalf("counters not folded in: %+v", got)
	}
	if len(fs.lastSpec.Peers) != 0 {
		t.Fatalf("peer over its quota must be gone from the endpoint, got %d", len(fs.lastSpec.Peers))
	}
}

// No stats route (an amnezia-box that predates it, or a proxy that is not
// answering) means no accounting this tick — never a guess, and never a
// fallback to the SQLite traffic history (spec Q11).
func TestSingboxSweepWithoutStatsRouteSkipsAccounting(t *testing.T) {
	ctx := context.Background()
	m, _, _ := newSingboxMgr(t)
	m.store.now = func() int64 { return 2000 }
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32", UsedRx: 7})
	m.SetPeerStats(func() (map[string]PeerStat, error) {
		return nil, ErrAwgPeerStatsUnsupported
	})

	m.SweepExpired(ctx)

	if got, _ := m.store.Get("P"); got.UsedRx != 7 || got.UsedTx != 0 {
		t.Fatalf("a failed fetch must leave the counters alone: %+v", got)
	}
}

// The dedupe gate mirrors traffic.Sampler's: a route that stays broken logs once,
// not every 30 seconds forever — and logs again once it breaks after a recovery,
// or the second outage of the day would be silent.
func TestSingboxSweepStatsErrorLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	ctx := context.Background()
	m, _, _ := newSingboxMgr(t)
	seedUsagePeer(t, m, Peer{PublicKey: "P", Address: "10.10.0.2/32"})
	fail := true
	m.SetPeerStats(func() (map[string]PeerStat, error) {
		if fail {
			return nil, errors.New("connection refused")
		}
		return map[string]PeerStat{"P": {RxBytes: 1}}, nil
	})

	m.SweepExpired(ctx)
	m.SweepExpired(ctx)
	if n := strings.Count(buf.String(), "connection refused"); n != 1 {
		t.Fatalf("a standing failure must log once, got %d:\n%s", n, buf.String())
	}
	m.mu.Lock()
	gated := m.lastUsageStatsErr
	m.mu.Unlock()
	if gated != "connection refused" {
		t.Fatalf("gate holds %q, want the recorded error", gated)
	}

	fail = false
	m.SweepExpired(ctx) // recovery re-arms the gate
	fail = true
	m.SweepExpired(ctx)
	if n := strings.Count(buf.String(), "connection refused"); n != 2 {
		t.Fatalf("a failure after a recovery must log again, got %d occurrences:\n%s", n, buf.String())
	}
}

// Lowering a quota below what is spent while the AWG server is DISABLED must
// still answer 200: the store has taken the new limit by design (it is not
// rolled back), and there is no interface to take the peer off — `awg set …
// remove` would only fail and turn a saved change into a 500. The peer is stored
// as out of service, which is all the next Enable needs: it renders the conf
// from peerLines, which skips it.
func TestSetPeerLimitsLoweringQuotaWithInterfaceDownIsNotAnError(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	m.store.now = func() int64 { return 1000 }
	seedUsagePeer(t, m, Peer{
		PublicKey: validPub, PresharedKey: "psk", Address: "10.10.0.2/32", Name: "bob",
		QuotaBytes: 4096, UsedRx: 500, UsedTx: 500,
	})
	// A missing interface fails BOTH the probe and any write against it — the
	// second one is what used to answer 500 on a change the store had kept.
	down := errors.New("Unable to access interface: No such device")
	f.errsContains["awg show"] = down
	f.errsContains["awg set awg-rb0 peer"] = down

	if err := m.SetPeerLimits(ctx, validPub, nil, i64(100)); err != nil {
		t.Fatalf("SetPeerLimits must succeed with the interface down: %v", err)
	}
	got, _ := m.store.Get(validPub)
	if got.QuotaBytes != 100 {
		t.Fatalf("the new quota must be stored: %+v", got)
	}
	if got.Suspension(1000) != quota.ReasonQuota {
		t.Fatalf("the peer must be stored out of service: %+v", got)
	}
	if f.sawContains("peer " + validPub + " remove") {
		t.Fatalf("nothing may be removed from an interface that is not there; calls=%v", f.calls)
	}
	// And the next Enable will not put it back: it is off the rendered conf.
	for _, pl := range m.peerLines() {
		if pl.PublicKey == validPub {
			t.Fatal("a suspended peer must not be rendered into the conf")
		}
	}
}

// Q11: no per-peer stats source at all means no accounting, and that must be
// visible in the log — the counters simply stop moving otherwise, and with them
// every quota. Once, through the same gate as a failing fetch.
func TestSingboxSweepWithoutStatsSourceLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	ctx := context.Background()
	m, _, _ := newSingboxMgr(t)
	seedUsagePeer(t, m, Peer{PublicKey: "P", Address: "10.10.0.2/32"})
	// no SetPeerStats: the source was never wired

	m.SweepExpired(ctx)
	if n := strings.Count(buf.String(), "per-peer stats source not wired"); n != 1 {
		t.Fatalf("a missing stats source must be reported once, got %d:\n%s", n, buf.String())
	}
	m.SweepExpired(ctx)
	if n := strings.Count(buf.String(), "per-peer stats source not wired"); n != 1 {
		t.Fatalf("the gate must not repeat it every tick, got %d:\n%s", n, buf.String())
	}
}

// The sweep's deltas are also the peer's history (#109): the observer gets
// exactly what the quota counters get, in the client's direction, and nothing
// on the priming tick.
func TestSweepReportsUsageDeltasToObserver(t *testing.T) {
	ctx := context.Background()
	f := newFakeRunner()
	m := newTestManager(t, f)
	seedConf(t, m)
	seedUsagePeer(t, m, Peer{PublicKey: "P", PresharedKey: "p", Address: "10.10.0.2/32"})
	var got []map[string]PeerUsage
	m.SetUsageObserver(func(d map[string]PeerUsage) { got = append(got, d) })

	f.outputs["awg show awg-rb0 transfer"] = "P\t100\t50\n"
	m.SweepExpired(ctx) // primes: nothing reported
	f.outputs["awg show awg-rb0 transfer"] = "P\t250\t80\n"
	m.SweepExpired(ctx)
	f.outputs["awg show awg-rb0 transfer"] = "P\t250\t80\n"
	m.SweepExpired(ctx) // no movement: nothing reported

	if len(got) != 1 {
		t.Fatalf("observer calls = %d, want 1 (%v)", len(got), got)
	}
	// rx 150 = what the peer SENT (upload); tx 30 = what it received.
	if u := got[0]["P"]; u.Up != 150 || u.Down != 30 {
		t.Fatalf("delta = %+v, want Up 150 Down 30", u)
	}
}

func TestLiveCountersKernelIsClientView(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	m.enabled = true
	f.outputs["awg show awg-rb0 transfer"] = "P\t700\t9000\n"
	got, err := m.LiveCounters(context.Background())
	if err != nil || got["P"] != (PeerUsage{Up: 700, Down: 9000}) {
		t.Fatalf("got %v err=%v", got, err)
	}
}

func TestLiveCountersDisabledServerIsNotAnError(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	m.enabled = false
	got, err := m.LiveCounters(context.Background())
	if err != nil || got != nil {
		t.Fatalf("got %v err=%v, want nil,nil", got, err)
	}
}

func TestLiveCountersSingboxReadsPeerStats(t *testing.T) {
	m := newTestManager(t, newFakeRunner())
	m.SetBackend("singbox")
	m.enabled = true
	m.SetPeerStats(func() (map[string]PeerStat, error) {
		return map[string]PeerStat{"P": {RxBytes: 3, TxBytes: 4}}, nil
	})
	got, err := m.LiveCounters(context.Background())
	if err != nil || got["P"] != (PeerUsage{Up: 3, Down: 4}) {
		t.Fatalf("got %v err=%v", got, err)
	}
	m.SetPeerStats(func() (map[string]PeerStat, error) { return nil, errors.New("down") })
	if _, err := m.LiveCounters(context.Background()); err == nil {
		t.Fatal("a failing stats route must surface as an error")
	}
}

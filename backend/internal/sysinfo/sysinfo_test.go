package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

const stat1 = "cpu  1000 0 500 8000 100 0 20 0 0 0\ncpu0 500 0 250 4000 50 0 10 0 0 0\ncpu1 500 0 250 4000 50 0 10 0 0 0\n"

// After stat1: +300 busy (user+system+irq), +700 idle (idle+iowait) => 30%.
const stat2 = "cpu  1200 0 580 8600 200 0 40 0 0 0\ncpu0 600 0 290 4300 100 0 20 0 0 0\ncpu1 600 0 290 4300 100 0 20 0 0 0\n"

const meminfo = "MemTotal:        4046924 kB\nMemFree:          812340 kB\nMemAvailable:    2301112 kB\nBuffers:          123456 kB\n"
const loadavg = "0.42 0.38 0.31 2/412 41218\n"
const pidStatus = "Name:\tamnezia-box\nUmask:\t0022\nVmPeak:\t  1200000 kB\nVmRSS:\t   98304 kB\nThreads:\t9\n"

func TestParseCPUStat(t *testing.T) {
	busy, idle, cores, err := parseCPUStat(stat1)
	if err != nil {
		t.Fatal(err)
	}
	if busy != 1000+500+20 || idle != 8000+100 || cores != 2 {
		t.Fatalf("busy=%d idle=%d cores=%d", busy, idle, cores)
	}
	if _, _, _, err := parseCPUStat("garbage\n"); err == nil {
		t.Fatal("want error on missing cpu line")
	}
}

func TestParseMeminfo(t *testing.T) {
	total, avail, err := parseMeminfo(meminfo)
	if err != nil || total != 4046924*1024 || avail != 2301112*1024 {
		t.Fatalf("total=%d avail=%d err=%v", total, avail, err)
	}
}

func TestParseLoadavg(t *testing.T) {
	l1, l5, l15, err := parseLoadavg(loadavg)
	if err != nil || l1 != 0.42 || l5 != 0.38 || l15 != 0.31 {
		t.Fatalf("%v %v %v %v", l1, l5, l15, err)
	}
}

func TestParseVmRSS(t *testing.T) {
	if rss := parseVmRSS(pidStatus); rss != 98304*1024 {
		t.Fatalf("rss = %d", rss)
	}
	if rss := parseVmRSS("Name:\tx\n"); rss != 0 {
		t.Fatalf("rss without VmRSS = %d, want 0", rss)
	}
}

func fakeProc(t *testing.T, stat string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0644)
	os.WriteFile(filepath.Join(dir, "meminfo"), []byte(meminfo), 0644)
	os.WriteFile(filepath.Join(dir, "loadavg"), []byte(loadavg), 0644)
	os.MkdirAll(filepath.Join(dir, "41218"), 0755)
	os.WriteFile(filepath.Join(dir, "41218", "status"), []byte(pidStatus), 0644)
	return dir
}

// CPU is a delta between two reads: the sampler remembers the previous one, so
// the first snapshot after start has nothing to compare against.
func TestSnapshotCPUFromDelta(t *testing.T) {
	dir := fakeProc(t, stat1)
	s := &Sampler{procRoot: dir, diskPath: dir}
	first := s.Snapshot(41218)
	if first.CPUPercent != nil {
		t.Fatalf("first snapshot must have no cpu_percent, got %v", *first.CPUPercent)
	}
	if first.Cores != 2 || first.MemTotal != 4046924*1024 || first.MemUsed != (4046924-2301112)*1024 {
		t.Fatalf("mem/cores: %+v", first)
	}
	if first.Load1 != 0.42 || first.ProcessRSS != 98304*1024 {
		t.Fatalf("load/rss: %+v", first)
	}
	if first.DiskTotal == 0 || first.DiskUsed > first.DiskTotal {
		t.Fatalf("disk: %+v", first)
	}
	os.WriteFile(filepath.Join(dir, "stat"), []byte(stat2), 0644)
	second := s.Snapshot(41218)
	if second.CPUPercent == nil || *second.CPUPercent != 30 {
		t.Fatalf("cpu_percent = %v, want 30", second.CPUPercent)
	}
}

// No pid (amnezia-box down) => no RSS, everything else still reported. A
// missing /proc file must not blow up the whole snapshot either.
func TestSnapshotDegradesGracefully(t *testing.T) {
	dir := fakeProc(t, stat1)
	os.Remove(filepath.Join(dir, "loadavg"))
	s := &Sampler{procRoot: dir, diskPath: dir}
	snap := s.Snapshot(0)
	if snap.ProcessRSS != 0 || snap.Load1 != 0 || snap.MemTotal == 0 {
		t.Fatalf("%+v", snap)
	}
}

// Package sysinfo reads host metrics for the dashboard straight from /proc and
// statfs: CPU load, memory, load average, the managed process's RSS and the
// root filesystem. No dependencies, Linux only — the same assumption the rest
// of RouteBox makes (/proc is already read for the process status).
package sysinfo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// Snapshot is one reading. Sizes are bytes. CPUPercent is nil on the first
// reading after start: it is a delta between two /proc/stat reads.
type Snapshot struct {
	CPUPercent *float64 `json:"cpu_percent"`
	Cores      int      `json:"cores"`
	Load1      float64  `json:"load1"`
	Load5      float64  `json:"load5"`
	Load15     float64  `json:"load15"`
	MemTotal   uint64   `json:"mem_total"`
	MemUsed    uint64   `json:"mem_used"` // MemTotal - MemAvailable, what `free` calls used
	ProcessRSS uint64   `json:"process_rss"`
	DiskTotal  uint64   `json:"disk_total"`
	DiskUsed   uint64   `json:"disk_used"`
}

// Sampler keeps the previous /proc/stat counters so consecutive snapshots
// yield a CPU percentage. Zero value reads the real /proc and "/".
type Sampler struct {
	mu       sync.Mutex
	prevBusy uint64
	prevIdle uint64
	primed   bool
	procRoot string // "" => /proc (tests point it at a fixture dir)
	diskPath string // "" => "/"
}

func (s *Sampler) proc(name string) string {
	root := s.procRoot
	if root == "" {
		root = "/proc"
	}
	return filepath.Join(root, name)
}

// Snapshot reads everything it can; a file that cannot be read leaves its
// fields zero rather than failing the whole reading — the dashboard shows "—"
// for that one metric. pid 0 means no managed process.
func (s *Sampler) Snapshot(pid int) Snapshot {
	var snap Snapshot
	if data, err := os.ReadFile(s.proc("stat")); err == nil {
		if busy, idle, cores, err := parseCPUStat(string(data)); err == nil {
			snap.Cores = cores
			s.mu.Lock()
			if s.primed {
				db, di := busy-s.prevBusy, idle-s.prevIdle
				if db+di > 0 {
					pct := float64(db) / float64(db+di) * 100
					snap.CPUPercent = &pct
				}
			}
			s.prevBusy, s.prevIdle, s.primed = busy, idle, true
			s.mu.Unlock()
		}
	}
	if data, err := os.ReadFile(s.proc("meminfo")); err == nil {
		if total, avail, err := parseMeminfo(string(data)); err == nil {
			snap.MemTotal, snap.MemUsed = total, total-avail
		}
	}
	if data, err := os.ReadFile(s.proc("loadavg")); err == nil {
		snap.Load1, snap.Load5, snap.Load15, _ = parseLoadavg(string(data))
	}
	if pid > 0 {
		if data, err := os.ReadFile(s.proc(fmt.Sprintf("%d/status", pid))); err == nil {
			snap.ProcessRSS = parseVmRSS(string(data))
		}
	}
	path := s.diskPath
	if path == "" {
		path = "/"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err == nil {
		snap.DiskTotal = st.Blocks * uint64(st.Bsize)
		snap.DiskUsed = (st.Blocks - st.Bfree) * uint64(st.Bsize)
	}
	return snap
}

// parseCPUStat returns the aggregate busy/idle jiffies from the "cpu" line and
// the number of "cpuN" lines. Busy = user+nice+system+irq+softirq+steal,
// idle = idle+iowait — the split top/htop use.
func parseCPUStat(text string) (busy, idle uint64, cores int, err error) {
	found := false
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			cores++
			continue
		}
		var v [10]uint64
		for i := 1; i < len(f) && i <= 10; i++ {
			v[i-1], _ = strconv.ParseUint(f[i], 10, 64)
		}
		busy = v[0] + v[1] + v[2] + v[5] + v[6] + v[7]
		idle = v[3] + v[4]
		found = true
	}
	if !found {
		return 0, 0, 0, errors.New("no cpu line in /proc/stat")
	}
	return busy, idle, cores, nil
}

func parseMeminfo(text string) (total, available uint64, err error) {
	kb := func(line string) uint64 {
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0
		}
		n, _ := strconv.ParseUint(f[1], 10, 64)
		return n * 1024
	}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = kb(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = kb(line)
		}
	}
	if total == 0 {
		return 0, 0, errors.New("no MemTotal in /proc/meminfo")
	}
	return total, available, nil
}

func parseLoadavg(text string) (l1, l5, l15 float64, err error) {
	f := strings.Fields(text)
	if len(f) < 3 {
		return 0, 0, 0, errors.New("short /proc/loadavg")
	}
	l1, _ = strconv.ParseFloat(f[0], 64)
	l5, _ = strconv.ParseFloat(f[1], 64)
	l15, _ = strconv.ParseFloat(f[2], 64)
	return l1, l5, l15, nil
}

// parseVmRSS returns the resident set size from /proc/<pid>/status, 0 if absent.
func parseVmRSS(text string) uint64 {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				n, _ := strconv.ParseUint(f[1], 10, 64)
				return n * 1024
			}
		}
	}
	return 0
}

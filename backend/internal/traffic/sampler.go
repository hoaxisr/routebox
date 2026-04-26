package traffic

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// ConnectionSample is a snapshot of one connection at sample time.
type ConnectionSample struct {
	ID       string
	Source   string
	Domain   string
	Chain    string
	Upload   int64
	Download int64
}

// Delta is the aggregated bytes attributed to one (source, domain, chain) key
// since the previous sample tick.
type Delta struct {
	Source   string
	Domain   string
	Chain    string
	Upload   int64
	Download int64
}

type connState struct {
	source   string
	domain   string
	chain    string
	upload   int64
	download int64
}

// Sampler turns periodic Clash connection snapshots into per-minute byte deltas.
type Sampler struct {
	store    *Store
	mu       sync.Mutex
	lastSeen map[string]connState // connID → last counters
}

func NewSampler(store *Store) *Sampler {
	return &Sampler{
		store:    store,
		lastSeen: map[string]connState{},
	}
}

// computeDeltas computes per-key byte deltas vs the last snapshot. Connections
// missing from the new snapshot are evicted (their final delta is implicitly 0
// — the previous tick already accounted for everything we knew about them).
// Counter resets (value going backwards) are treated as a fresh start.
func (s *Sampler) computeDeltas(snapshot []ConnectionSample) []Delta {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := make(map[string]bool, len(snapshot))
	out := make([]Delta, 0, len(snapshot))
	for _, c := range snapshot {
		keep[c.ID] = true
		prev, ok := s.lastSeen[c.ID]
		var dUp, dDown int64
		if ok {
			if c.Upload >= prev.upload {
				dUp = c.Upload - prev.upload
			} else {
				dUp = c.Upload
			}
			if c.Download >= prev.download {
				dDown = c.Download - prev.download
			} else {
				dDown = c.Download
			}
		} else {
			dUp = c.Upload
			dDown = c.Download
		}
		s.lastSeen[c.ID] = connState{c.Source, c.Domain, c.Chain, c.Upload, c.Download}
		if dUp == 0 && dDown == 0 {
			continue
		}
		out = append(out, Delta{Source: c.Source, Domain: c.Domain, Chain: c.Chain, Upload: dUp, Download: dDown})
	}
	// Evict connections no longer present
	for id := range s.lastSeen {
		if !keep[id] {
			delete(s.lastSeen, id)
		}
	}
	return out
}

// fetchSnapshot pulls /connections from Clash and converts to our shape.
func (s *Sampler) fetchSnapshot(clashAddr string) ([]ConnectionSample, error) {
	resp, err := http.Get("http://" + clashAddr + "/connections")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data struct {
		Connections []struct {
			ID       string   `json:"id"`
			Upload   int64    `json:"upload"`
			Download int64    `json:"download"`
			Chains   []string `json:"chains"`
			Metadata struct {
				SourceIP      string `json:"sourceIP"`
				Host          string `json:"host"`
				DestinationIP string `json:"destinationIP"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	out := make([]ConnectionSample, len(data.Connections))
	for i, c := range data.Connections {
		domain := c.Metadata.Host
		if domain == "" {
			domain = c.Metadata.DestinationIP
		}
		if domain == "" {
			domain = "-"
		}
		chain := "-"
		if len(c.Chains) > 0 {
			chain = ""
			for j, ch := range c.Chains {
				if j > 0 {
					chain += " → "
				}
				chain += ch
			}
		}
		out[i] = ConnectionSample{
			ID:       c.ID,
			Source:   c.Metadata.SourceIP,
			Domain:   domain,
			Chain:    chain,
			Upload:   c.Upload,
			Download: c.Download,
		}
	}
	return out, nil
}

// Run starts the periodic sample → upsert loop. Stops when stop channel closes.
// retentionDays controls how long history is kept (0 = no pruning).
func (s *Sampler) Run(clashAddr string, retentionDays int, stop <-chan struct{}) {
	if s.store == nil {
		return
	}
	tickInterval := 60 * time.Second
	pruneInterval := 1 * time.Hour
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()
	prune := time.NewTicker(pruneInterval)
	defer prune.Stop()

	doSample := func() {
		snap, err := s.fetchSnapshot(clashAddr)
		if err != nil {
			return
		}
		deltas := s.computeDeltas(snap)
		now := time.Now().Unix()
		bucket := (now / 60) * 60
		for _, d := range deltas {
			if err := s.store.Upsert(bucket, d.Source, d.Domain, d.Chain, d.Upload, d.Download); err != nil {
				log.Printf("traffic upsert: %v", err)
			}
		}
	}
	doPrune := func() {
		if retentionDays <= 0 {
			return
		}
		cutoff := time.Now().Unix() - int64(retentionDays)*86400
		if err := s.store.PruneOlderThan(cutoff); err != nil {
			log.Printf("traffic prune: %v", err)
		}
	}

	for {
		select {
		case <-tick.C:
			doSample()
		case <-prune.C:
			doPrune()
		case <-stop:
			return
		}
	}
}

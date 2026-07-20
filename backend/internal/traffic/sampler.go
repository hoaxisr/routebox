package traffic

import (
	"encoding/json"
	"fmt"
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

// clashHTTPClient bounds Clash API calls so a hung socket can't stall the
// sampler loop indefinitely.
var clashHTTPClient = &http.Client{Timeout: 10 * time.Second}

// Sampler turns periodic Clash connection snapshots into per-minute byte deltas.
type Sampler struct {
	store    *Store
	mu       sync.Mutex
	lastSeen map[string]connState // connID → last counters

	// lastSampleErr dedupes fetch-error logging: only transitions (ok→error,
	// or a different error text) are logged, so a persistent failure — e.g.
	// the Clash secret changed in the panel without a RouteBox restart,
	// yielding endless 401s — produces one diagnostic line, not one per tick.
	// Touched only from the single Run goroutine; no locking needed.
	lastSampleErr string
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
// A non-empty secret is sent as a Bearer token (Clash API auth); a non-200
// response is a hard error — a 401 body must never decode into an empty
// snapshot that silently records zero traffic forever.
func (s *Sampler) fetchSnapshot(clashAddr, secret string) ([]ConnectionSample, error) {
	req, err := http.NewRequest("GET", "http://"+clashAddr+"/connections", nil)
	if err != nil {
		return nil, err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := clashHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clash /connections: status %d", resp.StatusCode)
	}
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

// noteSampleErr logs a fetch error only when it differs from the last one
// logged (see lastSampleErr). Returns true if it logged. Only ever called
// from the single Run goroutine.
func (s *Sampler) noteSampleErr(err error) bool {
	msg := err.Error()
	if msg == s.lastSampleErr {
		return false
	}
	s.lastSampleErr = msg
	log.Printf("traffic sampler: %v", err)
	return true
}

// Run starts the periodic sample → upsert loop. Stops when stop channel closes.
// retentionDays controls how long history is kept (0 = no pruning).
func (s *Sampler) Run(clashAddr, secret string, retentionDays int, stop <-chan struct{}) {
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
		snap, err := s.fetchSnapshot(clashAddr, secret)
		if err != nil {
			s.noteSampleErr(err)
			return
		}
		s.lastSampleErr = "" // success — a later failure should log again
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

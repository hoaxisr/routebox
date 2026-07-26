package traffic

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS traffic_minute (
  bucket_ts INTEGER NOT NULL,
  source    TEXT NOT NULL,
  domain    TEXT NOT NULL,
  chain     TEXT NOT NULL,
  upload    INTEGER NOT NULL,
  download  INTEGER NOT NULL,
  PRIMARY KEY (bucket_ts, source, domain, chain)
);
CREATE INDEX IF NOT EXISTS idx_traffic_bucket_ts ON traffic_minute(bucket_ts);
`

// AggRow is one aggregated bucket of traffic.
type AggRow struct {
	Source   string `json:"source"`
	Domain   string `json:"domain"`
	Chain    string `json:"chain"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

// Store is a thin wrapper around SQLite for traffic_minute.
type Store struct {
	db *sql.DB
}

// OpenStore opens (or creates) the SQLite file and runs schema migration.
func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(userSchema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// DeleteSource removes all traffic_minute rows for a source (LAN/tunnel IP). Used to
// purge a deleted client's per-source Breakdown history — and subtract it from any
// aggregate that sums traffic_minute — when the client is removed (#19).
func (s *Store) DeleteSource(source string) error {
	if source == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM traffic_minute WHERE source = ?`, source)
	return err
}

// Upsert adds upload/download counts to the row at (bucket_ts, source, domain, chain).
func (s *Store) Upsert(bucketTs int64, source, domain, chain string, upload, download int64) error {
	_, err := s.db.Exec(`
		INSERT INTO traffic_minute (bucket_ts, source, domain, chain, upload, download)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (bucket_ts, source, domain, chain)
		DO UPDATE SET upload = upload + excluded.upload, download = download + excluded.download
	`, bucketTs, source, domain, chain, upload, download)
	return err
}

// QueryAggregate sums upload/download grouped by (source, domain, chain) within
// the time window. Empty filter strings disable that filter.
func (s *Store) QueryAggregate(startTs, endTs int64, source, domain, chain string) ([]AggRow, error) {
	args := []interface{}{startTs, endTs}
	q := `SELECT source, domain, chain, SUM(upload), SUM(download)
	      FROM traffic_minute
	      WHERE bucket_ts >= ? AND bucket_ts <= ?`
	if source != "" {
		q += " AND source = ?"
		args = append(args, source)
	}
	if domain != "" {
		// Match the apex itself or any subdomain. Callers pass apex strings
		// (the UI uses ApexDomain to construct keys); raw rows in the DB are
		// full hostnames, so we expand on read.
		q += " AND (domain = ? OR domain LIKE ?)"
		args = append(args, domain, "%."+domain)
	}
	if chain != "" {
		q += " AND chain = ?"
		args = append(args, chain)
	}
	q += " GROUP BY source, domain, chain ORDER BY (SUM(upload) + SUM(download)) DESC"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AggRow
	for rows.Next() {
		var r AggRow
		if err := rows.Scan(&r.Source, &r.Domain, &r.Chain, &r.Upload, &r.Download); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneOlderThan deletes rows with bucket_ts < cutoff.
func (s *Store) PruneOlderThan(cutoffTs int64) error {
	_, err := s.db.Exec(`DELETE FROM traffic_minute WHERE bucket_ts < ?`, cutoffTs)
	return err
}

// Reset clears all accumulated traffic history. The schema is preserved so
// subsequent Upserts continue to work. The Sampler's in-memory lastSeen state
// is intentionally not touched — future deltas keep being computed against
// current Clash counters from this point forward.
func (s *Store) Reset() error {
	_, err := s.db.Exec(`DELETE FROM traffic_minute`)
	return err
}

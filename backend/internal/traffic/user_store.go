package traffic

import "strings"

// userSchema is applied by OpenStore alongside the connection-level schema.
const userSchema = `
CREATE TABLE IF NOT EXISTS user_traffic (
  bucket_ts INTEGER NOT NULL,
  user      TEXT NOT NULL,
  upload    INTEGER NOT NULL,
  download  INTEGER NOT NULL,
  PRIMARY KEY (bucket_ts, user)
);
CREATE INDEX IF NOT EXISTS idx_user_traffic_user_ts ON user_traffic(user, bucket_ts);
`

// UserHistoryRow is one per-minute bucket of a single user's traffic.
type UserHistoryRow struct {
	BucketTs int64 `json:"ts"`
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

// UpsertUser adds upload/download to the (bucket_ts, user) row.
func (s *Store) UpsertUser(bucketTs int64, user string, upload, download int64) error {
	_, err := s.db.Exec(`
		INSERT INTO user_traffic (bucket_ts, user, upload, download)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (bucket_ts, user)
		DO UPDATE SET upload = upload + excluded.upload, download = download + excluded.download
	`, bucketTs, user, upload, download)
	return err
}

// QueryUserTotals sums one user's upload/download over [startTs, endTs].
func (s *Store) QueryUserTotals(startTs, endTs int64, user string) (upload, download int64, err error) {
	err = s.db.QueryRow(`
		SELECT COALESCE(SUM(upload),0), COALESCE(SUM(download),0)
		FROM user_traffic WHERE user = ? AND bucket_ts >= ? AND bucket_ts <= ?
	`, user, startTs, endTs).Scan(&upload, &download)
	return
}

// QueryUserHistory returns one row per bucket for the user, ascending by time.
// Distinct from QueryUserTotals (which collapses time): this preserves the
// per-bucket series for sparklines.
func (s *Store) QueryUserHistory(startTs, endTs int64, user string) ([]UserHistoryRow, error) {
	rows, err := s.db.Query(`
		SELECT bucket_ts, upload, download FROM user_traffic
		WHERE user = ? AND bucket_ts >= ? AND bucket_ts <= ?
		ORDER BY bucket_ts ASC
	`, user, startTs, endTs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserHistoryRow
	for rows.Next() {
		var r UserHistoryRow
		if err := rows.Scan(&r.BucketTs, &r.Upload, &r.Download); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneUserOlderThan deletes user_traffic rows with bucket_ts < cutoff.
func (s *Store) PruneUserOlderThan(cutoffTs int64) error {
	_, err := s.db.Exec(`DELETE FROM user_traffic WHERE bucket_ts < ?`, cutoffTs)
	return err
}

// ResetUsers clears all per-user traffic history (schema preserved).
func (s *Store) ResetUsers() error {
	_, err := s.db.Exec(`DELETE FROM user_traffic`)
	return err
}

// DeleteUsers removes all traffic history for the given user names (the Breakdown
// series a deleted client leaves behind). No-op on an empty list.
func (s *Store) DeleteUsers(names []string) error {
	if len(names) == 0 {
		return nil
	}
	ph := make([]string, len(names))
	args := make([]interface{}, len(names))
	for i, n := range names {
		ph[i] = "?"
		args[i] = n
	}
	_, err := s.db.Exec(`DELETE FROM user_traffic WHERE user IN (`+strings.Join(ph, ",")+`)`, args...)
	return err
}

// keyArgs builds the "?,?,…" list and args for an IN clause over keys.
func keyArgs(keys []string) (string, []interface{}) {
	ph := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		ph[i] = "?"
		args[i] = k
	}
	return strings.Join(ph, ","), args
}

// QueryKeysTotals sums upload/download over several user_traffic keys in
// [startTs, endTs] — one consumer can be accounted under more than one key (a
// panel user with bindings under different names). No keys → 0/0.
func (s *Store) QueryKeysTotals(startTs, endTs int64, keys []string) (upload, download int64, err error) {
	if len(keys) == 0 {
		return 0, 0, nil
	}
	ph, args := keyArgs(keys)
	err = s.db.QueryRow(`SELECT COALESCE(SUM(upload),0), COALESCE(SUM(download),0) FROM user_traffic
		WHERE user IN (`+ph+`) AND bucket_ts >= ? AND bucket_ts <= ?`,
		append(args, startTs, endTs)...).Scan(&upload, &download)
	return
}

// QueryKeysHistory is QueryKeysTotals as a time series, summed over the keys
// and coarsened like QuerySourceHistory (HistoryStep) so a month stays bounded.
func (s *Store) QueryKeysHistory(startTs, endTs int64, keys []string) ([]UserHistoryRow, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	ph, args := keyArgs(keys)
	rows, err := s.db.Query(`SELECT MIN(bucket_ts), SUM(upload), SUM(download) FROM user_traffic
		WHERE user IN (`+ph+`) AND bucket_ts >= ? AND bucket_ts <= ?
		GROUP BY bucket_ts / ? ORDER BY 1 ASC`,
		append(args, startTs, endTs, HistoryStep(endTs-startTs))...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserHistoryRow
	for rows.Next() {
		var r UserHistoryRow
		if err := rows.Scan(&r.BucketTs, &r.Upload, &r.Download); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

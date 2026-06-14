package traffic

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

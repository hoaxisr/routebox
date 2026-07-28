package traffic

// Per-source (LAN/tunnel IP) reads over traffic_minute. The per-USER pair in
// user_store.go answers the same questions for sing-box inbound users, whose
// bytes come from the v2ray StatsService. An AWG peer is not an inbound user and
// has no counter there — its traffic is only ever seen as connections from its
// tunnel IP, which is exactly the `source` column here (#40).

// QuerySourceTotals sums one source's upload/download over [startTs, endTs].
// A source with no rows is 0/0, not an error.
func (s *Store) QuerySourceTotals(startTs, endTs int64, source string) (upload, download int64, err error) {
	err = s.db.QueryRow(`
		SELECT COALESCE(SUM(upload),0), COALESCE(SUM(download),0)
		FROM traffic_minute WHERE source = ? AND bucket_ts >= ? AND bucket_ts <= ?
	`, source, startTs, endTs).Scan(&upload, &download)
	return
}

// maxHistoryPoints caps one source's series. 1440 = a full day at minute
// resolution, so the default 24h view is unchanged and only longer ranges
// coarsen. Uncapped, a month of an always-on peer is ~43k points (~2 MB of
// JSON) — and the peers endpoint returns EVERY peer in one response.
const maxHistoryPoints = 1440

// historyStep returns the bucket width, in seconds, for a window: minute
// buckets until that would exceed maxHistoryPoints, then whole minutes coarse
// enough to stay under it. PURE.
func historyStep(window int64) int64 {
	if window <= 0 {
		return 60
	}
	step := window / maxHistoryPoints
	if step <= 60 {
		return 60
	}
	return step - step%60 // whole minutes: buckets are minute-aligned
}

// QuerySourceHistory returns the source's traffic as a time-ascending series.
// Rows are collapsed across domain/chain — traffic_minute is keyed by
// (bucket_ts, source, domain, chain), so a single minute of one peer's browsing
// is many rows and the sparkline wants one point per bucket — and, for long
// ranges, across several minutes as well (see historyStep). Each point is
// stamped with the first bucket it covers.
func (s *Store) QuerySourceHistory(startTs, endTs int64, source string) ([]UserHistoryRow, error) {
	step := historyStep(endTs - startTs)
	rows, err := s.db.Query(`
		SELECT MIN(bucket_ts), SUM(upload), SUM(download) FROM traffic_minute
		WHERE source = ? AND bucket_ts >= ? AND bucket_ts <= ?
		GROUP BY bucket_ts / ? ORDER BY 1 ASC
	`, source, startTs, endTs, step)
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

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

// LastSeenBySource returns, for every source with a bucket at or after `since`,
// the newest bucket it appears in. One query for the whole roster, because its
// caller (the AWG peer list on the sing-box backend) needs a timestamp per peer
// and would otherwise run a query per row.
//
// This is the closest thing to a handshake that backend has: sing-box serves AWG
// from a userspace endpoint, so there is no interface to ask and the only
// evidence a peer is there is that its tunnel IP moved bytes.
func (s *Store) LastSeenBySource(since int64) (map[string]int64, error) {
	rows, err := s.db.Query(`
		SELECT source, MAX(bucket_ts) FROM traffic_minute
		WHERE bucket_ts >= ? GROUP BY source
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var source string
		var ts int64
		if err := rows.Scan(&source, &ts); err != nil {
			return nil, err
		}
		out[source] = ts
	}
	return out, rows.Err()
}

// maxHistoryPoints caps one source's series. 1440 = a full day at minute
// resolution, so the default 24h view is unchanged and only longer ranges
// coarsen. Uncapped, a month of an always-on peer is ~43k points (~2 MB of
// JSON) — and the peers endpoint returns EVERY peer in one response.
const maxHistoryPoints = 1440

// HistoryStep returns the bucket width, in seconds, for a window: minute
// buckets until that would exceed maxHistoryPoints, then whole minutes coarse
// enough to stay under it. PURE.
func HistoryStep(window int64) int64 {
	if window <= 0 {
		return 60
	}
	step := window / maxHistoryPoints
	if step <= 60 {
		return 60
	}
	return step - step%60 // whole minutes: buckets are minute-aligned
}

// QuerySourceHistory returns the source's traffic as a time-ascending series;
// an empty source means every source — the whole network, for the dashboard
// graph (#99). Rows are collapsed across domain/chain — traffic_minute is keyed by
// (bucket_ts, source, domain, chain), so a single minute of one peer's browsing
// is many rows and the sparkline wants one point per bucket — and, for long
// ranges, across several minutes as well (see historyStep). Each point is
// stamped with the first bucket it covers.
func (s *Store) QuerySourceHistory(startTs, endTs int64, source string) ([]UserHistoryRow, error) {
	step := HistoryStep(endTs - startTs)
	q := `SELECT MIN(bucket_ts), SUM(upload), SUM(download) FROM traffic_minute
		WHERE bucket_ts >= ? AND bucket_ts <= ?`
	args := []interface{}{startTs, endTs}
	if source != "" {
		q += " AND source = ?"
		args = append(args, source)
	}
	q += " GROUP BY bucket_ts / ? ORDER BY 1 ASC"
	rows, err := s.db.Query(q, append(args, step)...)
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

// LeafHistoryRow is the download of one final outbound in one series bucket.
type LeafHistoryRow struct {
	BucketTs int64  `json:"ts"`
	Leaf     string `json:"leaf"`
	Download int64  `json:"download"`
}

// QueryLeafHistory is QuerySourceHistory's whole-network series split by the
// connection's final outbound — the first hop of the stored chain, since
// sing-box lists it leaf first. The dashboard sorts leaves into direct and
// proxied (#110); which tags are direct is config, not history, so it stays
// out of here. Rows are ascending by bucket, then by leaf.
func (s *Store) QueryLeafHistory(startTs, endTs int64) ([]LeafHistoryRow, error) {
	step := HistoryStep(endTs - startTs)
	rows, err := s.db.Query(`
		SELECT MIN(bucket_ts), leaf, SUM(download) FROM (
			SELECT bucket_ts, download, CASE WHEN instr(chain, ' → ') > 0
				THEN substr(chain, 1, instr(chain, ' → ') - 1) ELSE chain END AS leaf
			FROM traffic_minute WHERE bucket_ts >= ? AND bucket_ts <= ?
		) GROUP BY bucket_ts / ?, leaf ORDER BY 1 ASC, 2 ASC`, startTs, endTs, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeafHistoryRow
	for rows.Next() {
		var r LeafHistoryRow
		if err := rows.Scan(&r.BucketTs, &r.Leaf, &r.Download); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

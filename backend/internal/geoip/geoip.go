package geoip

import (
	"net"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// Info contains GeoIP information for an IP address
type Info struct {
	CountryCode   string `json:"country_code,omitempty"`
	Country       string `json:"country,omitempty"`
	Continent     string `json:"continent,omitempty"`
	ContinentCode string `json:"continent_code,omitempty"`
	ASN           string `json:"asn,omitempty"`
	ASName        string `json:"as_name,omitempty"`
	ASDomain      string `json:"as_domain,omitempty"`
}

// mmdbRecord is a combined struct that handles multiple MMDB formats:
// - IPLocate.io (country_code, country_name, continent_code)
// - IPInfo (country_code, country, continent, continent_code, asn, as_name, as_domain)
// - MaxMind GeoLite2 (similar structure)
type mmdbRecord struct {
	// Country fields (various formats)
	CountryCode string `maxminddb:"country_code"`
	CountryName string `maxminddb:"country_name"` // IPLocate format
	Country     string `maxminddb:"country"`      // IPInfo format

	// Continent
	Continent     string `maxminddb:"continent"`
	ContinentCode string `maxminddb:"continent_code"`

	// ASN (optional, not in all databases)
	ASN      string `maxminddb:"asn"`
	ASName   string `maxminddb:"as_name"`
	ASDomain string `maxminddb:"as_domain"`
}

// DB wraps MMDB database with thread-safe access
type DB struct {
	db *maxminddb.Reader
	mu sync.RWMutex
}

// Open opens a GeoIP database from the given path
func Open(path string) (*DB, error) {
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close closes the database
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Lookup returns GeoIP info for the given IP address
func (d *DB) Lookup(ipStr string) *Info {
	if d == nil || d.db == nil {
		return nil
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	var record mmdbRecord
	err := d.db.Lookup(ip, &record)
	if err != nil {
		return nil
	}

	// Return nil if no useful data found
	if record.CountryCode == "" {
		return nil
	}

	// Normalize country name (prefer country_name, fallback to country)
	countryName := record.CountryName
	if countryName == "" {
		countryName = record.Country
	}

	return &Info{
		CountryCode:   record.CountryCode,
		Country:       countryName,
		Continent:     record.Continent,
		ContinentCode: record.ContinentCode,
		ASN:           record.ASN,
		ASName:        record.ASName,
		ASDomain:      record.ASDomain,
	}
}

// IsLoaded returns true if database is loaded
func (d *DB) IsLoaded() bool {
	return d != nil && d.db != nil
}

package geo

import (
	_ "embed"
	"fmt"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

//go:embed GeoLite2-Country.mmdb
var MaxMindDB []byte

// ─── GeoIP ───────────────────────────────────────────────────────────────────
// GeoIP wraps the MaxMind GeoLite2-Country database.
//  maxminddb uses mmap internally so lookups are microsecond-latency 
// with no allocations on the hot path.
type GeoIP struct {
	db *maxminddb.Reader
}

// geoRecord is the subset of the MaxMind record we care about.
// maxminddb unmarshals only the fields present in this struct,
// so we pay no cost for the fields we don't need.
type geoRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"` // e.g. "DE", "US", "IE"
	} `maxminddb:"country"`
}

// NewGeoIP opens the embedded MaxMind database 
func NewGeoIP() (*GeoIP, error) {
	db, err := maxminddb.FromBytes(MaxMindDB)
	if err != nil {
		return nil, fmt.Errorf("open maxmind db: %w", err)
	}
	return &GeoIP{db: db}, nil
}

// LookupCountry returns the ISO 3166-1 alpha-2 country code for an IP address.
// Returns ("", nil) for private/loopback addresses — these are never
// sovereignty violations and we skip policy evaluation for them.
// Returns ("UNKNOWN", nil) if the IP is not in the database (rare).
func (g *GeoIP) LookupCountry(ipStr string) (string, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP: %q", ipStr)
	}

	// Skip RFC1918 private ranges, loopback, link-local.
	// These are intra-cluster or node-local connections — never a
	// cross-border data transfer.
	if isPrivate(ip) {
		return "", nil
	}

	var record geoRecord
	if err := g.db.Lookup(ip, &record); err != nil {
		return "", fmt.Errorf("maxmind lookup %s: %w", ipStr, err)
	}

	if record.Country.ISOCode == "" {
		return "UNKNOWN", nil
	}
	return record.Country.ISOCode, nil
}

// Close releases the MaxMind database file handle.
func (g *GeoIP) Close() error {
	return g.db.Close()
}

// ─── isPrivate ────────────────────────────────────────────────────────────────
// Returns true for addresses that are definitionally intra-network:
// loopback, RFC1918 private, link-local, and the K8s service CIDR (10.96.0.0/12).
var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16", // link-local
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, n, _ := net.ParseCIDR(cidr)
		nets = append(nets, n)
	}
	return nets
}()

func isPrivate(ip net.IP) bool {
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

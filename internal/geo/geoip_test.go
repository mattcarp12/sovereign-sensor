package geo

import (
	"testing"
)

func TestGeoIP_LookupCountry(t *testing.T) {
	geo, err := NewGeoIP()
	if err != nil {
		t.Fatalf("Failed to open MaxMind database: %v", err)
	}
	defer func() { _ = geo.Close() }()

	tests := []struct {
		name        string
		ip          string
		iso         string
		countryName string
		wantErr     bool
	}{
		{"Google DNS (US)", "8.8.8.8", "US", "United States", false},
		{"Localhost (Private)", "127.0.0.1", "", "", false},
		{"Internal K8s Network", "10.244.0.5", "", "", false},
		{"Invalid IP string", "not-an-ip", "", "", true},
		{"German IP", "46.243.125.53", "DE", "Germany", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iso, name, err := geo.LookupCountry(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("LookupCountry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if iso != tt.iso {
				t.Errorf("LookupCountry() = %v, want %v", iso, tt.iso)
			}

			if name != tt.countryName {
				t.Errorf("LookupCountry() = %v, want %v", name, tt.countryName)
			}
		})
	}
}

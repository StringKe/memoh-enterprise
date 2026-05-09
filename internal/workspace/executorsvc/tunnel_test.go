package executorsvc

import (
	"testing"
)

func TestValidateTunnelAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		address string
		wantErr bool
	}{
		{"loopback v4", "127.0.0.1:9222", false},
		{"loopback v6", "[::1]:9222", false},
		{"localhost", "localhost:9222", false},
		{"empty", "", true},
		{"missing port", "127.0.0.1", true},
		{"non loopback", "10.0.0.1:9222", true},
		{"public dns", "example.com:9222", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateTunnelAddress(tc.address)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateTunnelAddress(%q) returned nil, want error", tc.address)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateTunnelAddress(%q) returned %v, want nil", tc.address, err)
			}
		})
	}
}

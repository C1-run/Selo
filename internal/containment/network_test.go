package containment

import (
	"testing"
)

func TestValidateNetworkConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *NetworkConfig
		wantErr bool
	}{
		{
			name:    "nil config is valid",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "none mode is valid",
			config:  &NetworkConfig{Mode: NetworkModeNone},
			wantErr: false,
		},
		{
			name:    "all mode is valid",
			config:  &NetworkConfig{Mode: NetworkModeAll},
			wantErr: false,
		},
		{
			name: "whitelist mode with domains is valid",
			config: &NetworkConfig{
				Mode:           NetworkModeWhitelist,
				AllowedDomains: []string{"proxy.golang.org", "registry.npmjs.org"},
			},
			wantErr: false,
		},
		{
			name: "whitelist mode without domains is invalid",
			config: &NetworkConfig{
				Mode:           NetworkModeWhitelist,
				AllowedDomains: []string{},
			},
			wantErr: true,
		},
		{
			name: "whitelist mode with invalid domain is invalid",
			config: &NetworkConfig{
				Mode:           NetworkModeWhitelist,
				AllowedDomains: []string{"invalid domain with spaces"},
			},
			wantErr: true,
		},
		{
			name: "unknown mode is invalid",
			config: &NetworkConfig{
				Mode: "unknown",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNetworkConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		{"localhost", true},
		{"localhost:8080", true},
		{"example.com", true},
		{"proxy.golang.org", true},
		{"registry.npmjs.org", true},
		{"invalid domain", false},
		{"", false},
		{"no-dot", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			if got := isValidDomain(tt.domain); got != tt.valid {
				t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, got, tt.valid)
			}
		})
	}
}

func TestContainsDomain(t *testing.T) {
	tests := []struct {
		hostname       string
		allowedDomains []string
		expected       bool
	}{
		{"proxy.golang.org", []string{"proxy.golang.org"}, true},
		{"sub.proxy.golang.org", []string{"proxy.golang.org"}, true},
		{"other.com", []string{"proxy.golang.org"}, false},
		{"localhost", []string{"localhost"}, true},
		{"localhost:8080", []string{"localhost"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			if got := containsDomain(tt.hostname, tt.allowedDomains); got != tt.expected {
				t.Errorf("containsDomain(%q, %v) = %v, want %v", tt.hostname, tt.allowedDomains, got, tt.expected)
			}
		})
	}
}

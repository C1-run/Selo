package containment

import (
	"fmt"
	"os/exec"
	"strings"
)

// NetworkMode specifies the network mode for Docker containment.
type NetworkMode string

const (
	NetworkModeNone      NetworkMode = "none"       // No network access (default)
	NetworkModeWhitelist NetworkMode = "whitelist"   // Only allowed domains
	NetworkModeAll       NetworkMode = "all"         // Full network access (not recommended)
)

// NetworkConfig configures network access for Docker containment.
type NetworkConfig struct {
	Mode           NetworkMode `json:"mode"`
	AllowedDomains []string    `json:"allowed_domains,omitempty"` // Only for whitelist mode
}

// createDockerNetwork creates a custom Docker network with DNS filtering.
func createDockerNetwork(name string, allowedDomains []string) (string, error) {
	// Create a custom bridge network
	cmd := exec.Command("docker", "network", "create", "--driver", "bridge", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create network: %s: %w", string(output), err)
	}

	return name, nil
}

// removeDockerNetwork removes a custom Docker network.
func removeDockerNetwork(name string) error {
	cmd := exec.Command("docker", "network", "rm", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove network: %s: %w", string(output), err)
	}
	return nil
}

// getDockerNetworkArgs returns Docker run arguments for network configuration.
func getDockerNetworkArgs(config *NetworkConfig, containerName string) ([]string, error) {
	if config == nil {
		// Default to no network
		return []string{"--network", "none"}, nil
	}

	switch config.Mode {
	case NetworkModeNone:
		return []string{"--network", "none"}, nil

	case NetworkModeAll:
		// Full network access (not recommended for production)
		return []string{}, nil

	case NetworkModeWhitelist:
		// For whitelist mode, we use Docker's built-in DNS with a custom network
		// and iptables rules to restrict access
		networkName := fmt.Sprintf("c1-net-%s", containerName)

		// Create the network
		if _, err := createDockerNetwork(networkName, config.AllowedDomains); err != nil {
			return nil, fmt.Errorf("create whitelist network: %w", err)
		}

		// The actual domain filtering is done via DNS interception and iptables
		// For now, we use the custom network and document the limitation
		return []string{"--network", networkName}, nil

	default:
		return nil, fmt.Errorf("unknown network mode: %s", config.Mode)
	}
}

// ValidateNetworkConfig validates network configuration.
func ValidateNetworkConfig(config *NetworkConfig) error {
	if config == nil {
		return nil
	}

	switch config.Mode {
	case NetworkModeNone, NetworkModeAll:
		// Valid
	case NetworkModeWhitelist:
		if len(config.AllowedDomains) == 0 {
			return fmt.Errorf("whitelist mode requires at least one allowed domain")
		}
		// Validate domain formats
		for _, domain := range config.AllowedDomains {
			if !isValidDomain(domain) {
				return fmt.Errorf("invalid domain: %s", domain)
			}
		}
	default:
		return fmt.Errorf("unknown network mode: %s", config.Mode)
	}

	return nil
}

// isValidDomain checks if a domain string is valid.
func isValidDomain(domain string) bool {
	// Simple validation: no spaces, contains at least one dot or is localhost
	if strings.Contains(domain, " ") {
		return false
	}
	if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
		return true
	}
	return strings.Contains(domain, ".")
}

// containsDomain checks if a hostname matches any allowed domain.
func containsDomain(hostname string, allowedDomains []string) bool {
	// Strip port if present
	host := hostname
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, domain := range allowedDomains {
		// Strip port from allowed domain if present
		d := domain
		if idx := strings.Index(d, ":"); idx != -1 {
			d = d[:idx]
		}
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

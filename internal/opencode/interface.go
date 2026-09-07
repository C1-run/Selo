package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// OpenCodeAPI is the interface for OpenCode API operations.
// This abstraction allows for different implementations (REST, mock, etc.)
// and makes it easier to test and switch between backends.
type OpenCodeAPI interface {
	// CreateSession creates a new OpenCode session.
	CreateSession(ctx context.Context) (*Session, error)
	// Prompt sends a message to a session and returns the response.
	Prompt(ctx context.Context, sessionID, prompt string) (*PromptResponse, error)
	// GetMessages returns all messages in a session.
	GetMessages(ctx context.Context, sessionID string) ([]Message, error)
	// Abort stops a running prompt in a session.
	Abort(ctx context.Context, sessionID string) error
	// Close cleans up the client.
	Close() error
	// GetVersion returns the server version information.
	GetVersion(ctx context.Context) (*VersionInfo, error)
	// CheckCompatibility checks if the server version is compatible.
	CheckCompatibility(ctx context.Context) (*CompatibilityResult, error)
}

// VersionInfo contains version information about the OpenCode server.
type VersionInfo struct {
	Version    string `json:"version"`
	GoVersion  string `json:"go_version,omitempty"`
	BuildDate  string `json:"build_date,omitempty"`
	CommitHash string `json:"commit_hash,omitempty"`
}

// CompatibilityResult indicates whether the server version is compatible.
type CompatibilityResult struct {
	Compatible bool   `json:"compatible"`
	Current    string `json:"current"`
	Minimum    string `json:"minimum"`
	Message    string `json:"message,omitempty"`
}

// VersionedClient wraps a Client with version detection and compatibility checking.
type VersionedClient struct {
	*Client
	versionInfo     *VersionInfo
	compatibility   *CompatibilityResult
	checkedVersion  bool
}

// NewVersionedClient creates a new versioned OpenCode client.
func NewVersionedClient(baseURL string) *VersionedClient {
	return &VersionedClient{
		Client: NewClient(baseURL),
	}
}

// GetVersion returns the server version information.
func (vc *VersionedClient) GetVersion(ctx context.Context) (*VersionInfo, error) {
	// Try to get version from /version endpoint
	resp, err := vc.do(ctx, "GET", "/version", nil)
	if err != nil {
		// If endpoint doesn't exist, try to infer from binary
		return vc.inferVersion(ctx)
	}
	defer resp.Body.Close()

	var version VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, fmt.Errorf("decode version: %w", err)
	}

	vc.versionInfo = &version
	return &version, nil
}

// inferVersion attempts to get version information from the binary.
func (vc *VersionedClient) inferVersion(ctx context.Context) (*VersionInfo, error) {
	// This is a fallback when the /version endpoint doesn't exist
	// We'll return a minimal version info
	return &VersionInfo{
		Version: "unknown",
	}, nil
}

// CheckCompatibility checks if the server version is compatible.
func (vc *VersionedClient) CheckCompatibility(ctx context.Context) (*CompatibilityResult, error) {
	if vc.checkedVersion && vc.compatibility != nil {
		return vc.compatibility, nil
	}

	version, err := vc.GetVersion(ctx)
	if err != nil {
		return &CompatibilityResult{
			Compatible: false,
			Message:    fmt.Sprintf("failed to get version: %v", err),
		}, nil
	}

	// Minimum compatible version
	minimumVersion := "1.0.0"

	compatible := isVersionCompatible(version.Version, minimumVersion)
	result := &CompatibilityResult{
		Compatible: compatible,
		Current:    version.Version,
		Minimum:    minimumVersion,
	}

	if !compatible {
		result.Message = fmt.Sprintf("Server version %s is below minimum required %s", version.Version, minimumVersion)
	}

	vc.compatibility = result
	vc.checkedVersion = true
	return result, nil
}

// isVersionCompatible checks if the current version meets the minimum requirement.
func isVersionCompatible(current, minimum string) bool {
	// If current version is unknown, consider it compatible
	if current == "unknown" {
		return true
	}

	// Simple semver comparison
	currentParts := strings.Split(current, ".")
	minimumParts := strings.Split(minimum, ".")

	// Compare major version
	if len(currentParts) > 0 && len(minimumParts) > 0 {
		currentMajor := parseInt(currentParts[0])
		minimumMajor := parseInt(minimumParts[0])
		if currentMajor > minimumMajor {
			return true
		}
		if currentMajor < minimumMajor {
			return false
		}
	}

	// Compare minor version
	if len(currentParts) > 1 && len(minimumParts) > 1 {
		currentMinor := parseInt(currentParts[1])
		minimumMinor := parseInt(minimumParts[1])
		if currentMinor > minimumMinor {
			return true
		}
		if currentMinor < minimumMinor {
			return false
		}
	}

	// Compare patch version
	if len(currentParts) > 2 && len(minimumParts) > 2 {
		currentPatch := parseInt(currentParts[2])
		minimumPatch := parseInt(minimumParts[2])
		return currentPatch >= minimumPatch
	}

	// If versions are equal up to the length of the shorter one
	return true
}

// parseInt parses a string to int, returning 0 on error.
func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

// MockOpenCodeAPI is a mock implementation for testing.
type MockOpenCodeAPI struct {
	CreateSessionFn func(ctx context.Context) (*Session, error)
	PromptFn        func(ctx context.Context, sessionID, prompt string) (*PromptResponse, error)
	GetMessagesFn   func(ctx context.Context, sessionID string) ([]Message, error)
	AbortFn         func(ctx context.Context, sessionID string) error
	CloseFn         func() error
	GetVersionFn    func(ctx context.Context) (*VersionInfo, error)
}

// CreateSession implements OpenCodeAPI.
func (m *MockOpenCodeAPI) CreateSession(ctx context.Context) (*Session, error) {
	if m.CreateSessionFn != nil {
		return m.CreateSessionFn(ctx)
	}
	return &Session{ID: "mock-session"}, nil
}

// Prompt implements OpenCodeAPI.
func (m *MockOpenCodeAPI) Prompt(ctx context.Context, sessionID, prompt string) (*PromptResponse, error) {
	if m.PromptFn != nil {
		return m.PromptFn(ctx, sessionID, prompt)
	}
	return &PromptResponse{}, nil
}

// GetMessages implements OpenCodeAPI.
func (m *MockOpenCodeAPI) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
	if m.GetMessagesFn != nil {
		return m.GetMessagesFn(ctx, sessionID)
	}
	return nil, nil
}

// Abort implements OpenCodeAPI.
func (m *MockOpenCodeAPI) Abort(ctx context.Context, sessionID string) error {
	if m.AbortFn != nil {
		return m.AbortFn(ctx, sessionID)
	}
	return nil
}

// Close implements OpenCodeAPI.
func (m *MockOpenCodeAPI) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

// GetVersion implements OpenCodeAPI.
func (m *MockOpenCodeAPI) GetVersion(ctx context.Context) (*VersionInfo, error) {
	if m.GetVersionFn != nil {
		return m.GetVersionFn(ctx)
	}
	return &VersionInfo{
		Version:   "1.0.0",
		GoVersion: runtime.Version(),
	}, nil
}

// CheckCompatibility implements OpenCodeAPI.
func (m *MockOpenCodeAPI) CheckCompatibility(ctx context.Context) (*CompatibilityResult, error) {
	version, err := m.GetVersion(ctx)
	if err != nil {
		return nil, err
	}
	return &CompatibilityResult{
		Compatible: true,
		Current:    version.Version,
		Minimum:    "1.0.0",
	}, nil
}

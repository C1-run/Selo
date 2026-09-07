package opencode

import (
	"context"
	"testing"
)

func TestIsVersionCompatible(t *testing.T) {
	tests := []struct {
		current string
		minimum string
		want    bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"1.1.0", "1.0.0", true},
		{"2.0.0", "1.0.0", true},
		{"0.9.9", "1.0.0", false},
		{"1.0.0", "1.1.0", false},
		{"1.0.0", "2.0.0", false},
		{"1.0", "1.0.0", true},
		{"1", "1.0.0", true},
		{"unknown", "1.0.0", true}, // Unknown versions are considered compatible
	}

	for _, tt := range tests {
		t.Run(tt.current+"_"+tt.minimum, func(t *testing.T) {
			if got := isVersionCompatible(tt.current, tt.minimum); got != tt.want {
				t.Errorf("isVersionCompatible(%q, %q) = %v, want %v", tt.current, tt.minimum, got, tt.want)
			}
		})
	}
}

func TestMockOpenCodeAPI(t *testing.T) {
	mock := &MockOpenCodeAPI{}

	// Test CreateSession
	session, err := mock.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session == nil || session.ID == "" {
		t.Error("CreateSession() returned nil or empty session")
	}

	// Test Prompt
	resp, err := mock.Prompt(context.Background(), "session-1", "test prompt")
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp == nil {
		t.Error("Prompt() returned nil")
	}

	// Test GetVersion
	version, err := mock.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if version == nil || version.Version == "" {
		t.Error("GetVersion() returned nil or empty version")
	}

	// Test CheckCompatibility
	compat, err := mock.CheckCompatibility(context.Background())
	if err != nil {
		t.Fatalf("CheckCompatibility() error = %v", err)
	}
	if compat == nil || !compat.Compatible {
		t.Error("CheckCompatibility() returned nil or incompatible")
	}

	// Test Close
	if err := mock.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestMockOpenCodeAPI_CustomFunctions(t *testing.T) {
	customSession := &Session{ID: "custom-session"}
	customVersion := &VersionInfo{Version: "2.0.0"}

	mock := &MockOpenCodeAPI{
		CreateSessionFn: func(ctx context.Context) (*Session, error) {
			return customSession, nil
		},
		GetVersionFn: func(ctx context.Context) (*VersionInfo, error) {
			return customVersion, nil
		},
	}

	// Test custom CreateSession
	session, err := mock.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.ID != "custom-session" {
		t.Errorf("CreateSession() returned %v, want %v", session, customSession)
	}

	// Test custom GetVersion
	version, err := mock.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if version.Version != "2.0.0" {
		t.Errorf("GetVersion() returned %v, want %v", version, customVersion)
	}
}

// Package manifest defines the immutable RunManifest — the single source of
// truth for what an agent may and may not do in a run.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type RepoSpec struct {
	Path string `json:"path"`
	Base string `json:"base"` // base commit ref, default "HEAD"
}

type AgentSpec struct {
	Command []string `json:"command"`
	Image   string   `json:"image,omitempty"` // cell image; ignored by LocalCell
}

type FilesystemCapability struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

type ExecCapability struct {
	Allow []string `json:"allow,omitempty"`
}

type GitCapability struct {
	Commit bool `json:"commit"`
	Push   bool `json:"push"`
}

type NetworkCapability struct {
	Mode  string `json:"mode"` // "none" | "model"
	Model string `json:"model,omitempty"`
}

type NetworkPolicy struct {
	Connection NetworkCapability `json:"connection"`
}

type CapabilitySet struct {
	Filesystem FilesystemCapability `json:"filesystem"`
	Exec       ExecCapability       `json:"exec"`
	Git        GitCapability        `json:"git"`
}

type VerificationSpec struct {
	Test string `json:"test"`
}

type ResourceLimits struct {
	MaxMinutes int    `json:"max_minutes"`
	MaxLines   int    `json:"max_lines,omitempty"`
	MaxFiles   int    `json:"max_files,omitempty"`
	WorkMem    string `json:"work_mem,omitempty"` // docker mem limit, e.g. "2g"
}

type RunManifest struct {
	Version    string           `json:"version"`
	RunID      string           `json:"run_id"`
	Task       string           `json:"task"`
	Repo       RepoSpec         `json:"repo"`
	Agent      AgentSpec        `json:"agent"`
	Capability CapabilitySet    `json:"capabilities"`
	Network    NetworkPolicy    `json:"network"`
	Verify     VerificationSpec `json:"verification"`
	Limits     ResourceLimits   `json:"limits"`
}

// canonicalJSON marshals the manifest with deterministic field ordering.
func canonicalJSON(m *RunManifest) ([]byte, error) {
	// Struct field order is fixed by the type; slices keep order. Sort the
	// two glob lists so identical manifests hash identically.
	sortedCopy := *m
	sortedCopy.Capability.Filesystem.Read = sortedStrings(sortedCopy.Capability.Filesystem.Read)
	sortedCopy.Capability.Filesystem.Write = sortedStrings(sortedCopy.Capability.Filesystem.Write)
	sortedCopy.Capability.Exec.Allow = sortedStrings(sortedCopy.Capability.Exec.Allow)
	return json.MarshalIndent(&sortedCopy, "", "  ")
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// Frozen is a manifest sealed with its canonical hash.
type Frozen struct {
	Manifest *RunManifest `json:"manifest"`
	Hash     string       `json:"manifest_hash"`
}

// Freeze canonicalizes the manifest and binds it to its hash. After this
// point the manifest is immutable: any later mutation changes the hash.
func Freeze(m *RunManifest) (*Frozen, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest is nil")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("manifest.version is required")
	}
	if m.RunID == "" {
		return nil, fmt.Errorf("manifest.run_id is required")
	}
	if m.Repo.Path == "" {
		return nil, fmt.Errorf("manifest.repo.path is required")
	}
	if len(m.Agent.Command) == 0 {
		return nil, fmt.Errorf("manifest.agent.command is required")
	}
	canon, err := canonicalJSON(m)
	if err != nil {
		return nil, fmt.Errorf("canonicalize: %w", err)
	}
	sum := sha256.Sum256(canon)
	return &Frozen{Manifest: m, Hash: hex.EncodeToString(sum[:])}, nil
}

// Load reads and freezes a manifest from a JSON file.
func Load(path string, runID string) (*Frozen, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m RunManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if runID != "" {
		m.RunID = runID
	}
	return Freeze(&m)
}

// HashOf recomputes the canonical hash for verification.
func HashOf(m *RunManifest) string {
	canon, _ := canonicalJSON(m)
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])
}
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The dogfood config wraps ZCode runs in Selo. It must load, pass
// validateConfig, and advertise only keys Selo actually reads — a config that
// silently drops keys or gets refused at runtime defeats the dogfood purpose.
func TestDogfoodZcodeConfigLoadsAndValidates(t *testing.T) {
	data, err := os.ReadFile("../../config/selo.zcode.yaml")
	if err != nil {
		t.Fatalf("read dogfood config: %v", err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "selo.yaml"), data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := loadConfig("config/selo.yaml", dir)
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("dogfood config rejected by validateConfig: %v", err)
	}
	if cfg.Forge.Runner.Mode != "real" {
		t.Errorf("Runner.Mode = %q, want real", cfg.Forge.Runner.Mode)
	}
	if cfg.Forge.Runner.Command != "zcode" {
		t.Errorf("Runner.Command = %q, want zcode", cfg.Forge.Runner.Command)
	}
	if len(cfg.Forge.Runner.Args) == 0 || cfg.Forge.Runner.Args[0] != "exec" {
		t.Errorf("Runner.Args = %v, want exec template", cfg.Forge.Runner.Args)
	}
	if got := unknownConfigKeys(data, cfg); len(got) > 0 {
		t.Errorf("dogfood config advertises keys Selo does not read: %v", got)
	}
}

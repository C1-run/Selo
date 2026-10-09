package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Receipt helpers ---

// envTruthy reports whether an environment variable is set to a truthy value
// ("1", "true", "yes", "on"). Used for the opt-in switches Selo reads from the
// environment (e.g. SELO_TSA_SOFT).
func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// readNewestReceiptJSON reads the most recent JSON receipt from a directory.
func readNewestReceiptJSON(dir string) map[string]interface{} {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var newest string
	var newestMod int64
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			info, _ := e.Info()
			if info.ModTime().Unix() > newestMod {
				newestMod = info.ModTime().Unix()
				newest = filepath.Join(dir, e.Name())
			}
		}
	}
	if newest == "" {
		return nil
	}
	data, err := os.ReadFile(newest)
	if err != nil {
		return nil
	}
	var rec map[string]interface{}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil
	}
	return rec
}

// --- Smoke test framework ---

// SmokeCheck describes a single check in a smoke test suite.
type SmokeCheck struct {
	Desc string
	Fn   func() bool
}

// runSmokeSuite runs a list of checks and prints pass/fail results.
func runSmokeSuite(name string, checks []SmokeCheck) {
	fmt.Printf("=== Selo Smoke Test: %s ===\n", name)
	passed, failed := 0, 0
	for i, c := range checks {
		fmt.Printf("\n%d. %s...\n", i+1, c.Desc)
		if c.Fn() {
			passed++
		} else {
			failed++
		}
	}
	fmt.Printf("\n=== Result: %d passed, %d failed ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// createFixtureRepo creates a disposable git repo with the given files and returns its path.
// The caller should defer os.RemoveAll on the returned path.
func createFixtureRepo(prefix string, files map[string]string) string {
	repoDir, _ := os.MkdirTemp("", prefix+"-test-*")
	initGitRepo(repoDir)
	for name, content := range files {
		writeFile(filepath.Join(repoDir, name), content)
	}
	gitAddCommit(repoDir, "initial")
	return repoDir
}

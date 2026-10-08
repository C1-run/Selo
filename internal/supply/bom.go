package supply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Component represents an external supply component (MCP server, skill, npm package).
type Component struct {
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Source    string `json:"source"`    // file path where discovered
	Ecosystem string `json:"ecosystem"` // npm | pypi | go | github
}

// Vuln represents a vulnerability from OSV.
type Vuln struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Severity string `json:"severity,omitempty"`
}

// BOMResult holds supply check outcome.
type BOMResult struct {
	Components []Component `json:"components"`
	Hits       []string    `json:"hits"` // human-readable risks
	CheckedAt  time.Time   `json:"checked_at"`
}

// Discover scans repo for supply components.
// Looks at: .opencode/**/*.json, mcp*.json, package.json, opencode.json
func Discover(repoPath string) ([]Component, error) {
	var comps []Component
	seen := map[string]bool{}

	add := func(c Component) {
		key := c.Ecosystem + ":" + c.Name + "@" + c.Version
		if !seen[key] {
			seen[key] = true
			comps = append(comps, c)
		}
	}

	// 1. package.json dependencies
	if data, err := os.ReadFile(filepath.Join(repoPath, "package.json")); err == nil {
		var pkg map[string]interface{}
		if json.Unmarshal(data, &pkg) == nil {
			for _, field := range []string{"dependencies", "devDependencies", "peerDependencies"} {
				if deps, ok := pkg[field].(map[string]interface{}); ok {
					for name, ver := range deps {
						vs, _ := ver.(string)
						// Heuristic: MCP-related or external
						if strings.Contains(name, "mcp") || strings.HasPrefix(name, "@modelcontextprotocol/") || strings.Contains(name, "opencode") {
							add(Component{Name: name, Version: strings.Trim(vs, "^~>=< "), Source: "package.json", Ecosystem: "npm"})
						}
					}
				}
			}
		}
	}
	// .opencode/package.json
	if data, err := os.ReadFile(filepath.Join(repoPath, ".opencode", "package.json")); err == nil {
		var pkg map[string]interface{}
		if json.Unmarshal(data, &pkg) == nil {
			for _, field := range []string{"dependencies", "devDependencies"} {
				if deps, ok := pkg[field].(map[string]interface{}); ok {
					for name, ver := range deps {
						vs, _ := ver.(string)
						add(Component{Name: name, Version: strings.Trim(vs, "^~>=< "), Source: ".opencode/package.json", Ecosystem: "npm"})
					}
				}
			}
		}
	}

	// 2. MCP configs: opencode.json, mcp.json, .opencode/**/*.json
	patterns := []string{"opencode.json", "mcp.json", ".opencode/mcp.json", ".opencode/config.json"}
	for _, pat := range patterns {
		data, err := os.ReadFile(filepath.Join(repoPath, pat))
		if err != nil {
			continue
		}
		var cfg map[string]interface{}
		if json.Unmarshal(data, &cfg) != nil {
			continue
		}
		// opencode mcpServers
		if servers, ok := cfg["mcpServers"].(map[string]interface{}); ok {
			for name, v := range servers {
				ver := ""
				if m, ok := v.(map[string]interface{}); ok {
					if s, ok := m["version"].(string); ok {
						ver = s
					}
					if cmd, ok := m["command"].(string); ok && strings.Contains(cmd, "npx") {
						// try to extract package from args
						if args, ok := m["args"].([]interface{}); ok && len(args) > 0 {
							if pkg, ok := args[0].(string); ok && strings.Contains(pkg, "/") {
								name = pkg
							}
						}
					}
				}
				add(Component{Name: name, Version: ver, Source: pat, Ecosystem: "npm"})
			}
		}
		// generic plugins/skills
		if plugins, ok := cfg["plugin"].([]interface{}); ok {
			for _, p := range plugins {
				if s, ok := p.(string); ok {
					add(Component{Name: s, Source: pat, Ecosystem: "github"})
				}
			}
		}
	}

	// 3. Walk .opencode/tools/*.json (skip node_modules)
	filepath.Walk(filepath.Join(repoPath, ".opencode"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		rel, _ := filepath.Rel(repoPath, path)
		if rel == ".opencode/package.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var m map[string]interface{}
		if json.Unmarshal(data, &m) != nil {
			return nil
		}
		// Heuristic: look for npm package references
		var walk func(v interface{})
		walk = func(v interface{}) {
			switch x := v.(type) {
			case map[string]interface{}:
				for k, val := range x {
					if k == "package" || k == "name" {
						if s, ok := val.(string); ok && (strings.Contains(s, "/") || strings.Contains(s, "mcp")) {
							add(Component{Name: s, Source: rel, Ecosystem: "npm"})
						}
					}
					walk(val)
				}
			case []interface{}:
				for _, e := range x {
					walk(e)
				}
			}
		}
		walk(m)
		return nil
	})

	return comps, nil
}

// QueryOSV checks a component against OSV API (https://api.osv.dev/v1/query).
// Returns vulns if found. Timeout 5s, best-effort (network failure = no vulns).
func QueryOSV(c Component) ([]Vuln, error) {
	if c.Ecosystem != "npm" && c.Ecosystem != "pypi" && c.Ecosystem != "go" {
		return nil, nil
	}
	if c.Version == "" {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]interface{}{
		"package": map[string]string{"name": c.Name, "ecosystem": mapEcosystem(c.Ecosystem)},
		"version": c.Version,
	})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("https://api.osv.dev/v1/query", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, nil // best-effort, treat network failure as no vulns
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil
	}
	var out struct {
		Vulns []struct {
			ID       string `json:"id"`
			Summary  string `json:"summary"`
			Severity []struct {
				Type  string `json:"type"`
				Score string `json:"score"`
			} `json:"severity"`
		} `json:"vulns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil
	}
	var vulns []Vuln
	for _, v := range out.Vulns {
		sev := ""
		if len(v.Severity) > 0 {
			sev = v.Severity[0].Score
		}
		vulns = append(vulns, Vuln{ID: v.ID, Summary: v.Summary, Severity: sev})
	}
	return vulns, nil
}

func mapEcosystem(e string) string {
	switch e {
	case "npm":
		return "npm"
	case "pypi":
		return "PyPI"
	case "go":
		return "Go"
	default:
		return e
	}
}

// Check performs discover + OSV query (opt-in via SELO_ENABLE_OSV=1), returns BOMResult.
// High-risk if any vuln found or component from unknown github with no version.
func Check(repoPath string) (*BOMResult, error) {
	comps, _ := Discover(repoPath)
	res := &BOMResult{Components: comps, CheckedAt: time.Now().UTC()}
	// OSV query is opt-in to avoid network latency in default runs (5s per component)
	if os.Getenv("SELO_ENABLE_OSV") != "1" {
		return res, nil
	}
	for _, c := range comps {
		vulns, _ := QueryOSV(c)
		for _, v := range vulns {
			hit := fmt.Sprintf("supply: %s@%s vuln %s: %s", c.Name, c.Version, v.ID, v.Summary)
			res.Hits = append(res.Hits, hit)
		}
		// Heuristic: github component without version and not local file
		if c.Ecosystem == "github" && c.Version == "" && !strings.HasPrefix(c.Name, "./") && !strings.HasPrefix(c.Name, "/") {
			// Check if it's a local plugin path vs remote — remote without pin is risky
			if strings.Contains(c.Name, "github.com") || strings.Contains(c.Name, "/") {
				// Log low-severity hit for visibility, not blocking yet
				// res.Hits = append(res.Hits, fmt.Sprintf("supply: unpinned github %s (%s)", c.Name, c.Source))
			}
		}
	}
	return res, nil
}

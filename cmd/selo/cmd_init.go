package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initRepo string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a Selo workspace",
	Long: `Creates the necessary directory structure and default configuration for
Selo in the specified repository (or current directory).`,
	RunE: runInitCmd,
}

func init() {
	initCmd.Flags().StringVar(&initRepo, "repo", "", "Repository path (default: cwd)")
}

func runInitCmd(cmd *cobra.Command, args []string) error {
	if initRepo == "" {
		var err error
		initRepo, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine cwd: %w", err)
		}
	}

	// These are the directories the queue manager and receipt writer actually
	// use at runtime; a parallel tree under .selo/ would never be touched.
	dirs := []string{
		"queue/pending",
		"queue/running",
		"queue/review",
		"queue/done",
		"queue/failed",
		"runs",
		"receipts",
		"worktrees",
		"config",
	}

	for _, d := range dirs {
		full := filepath.Join(initRepo, d)
		if err := os.MkdirAll(full, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
		fmt.Printf("  created %s/\n", d)
	}

	// Write default config
	configPath := filepath.Join(initRepo, "config", "selo.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := `# Selo configuration
forge:
  poll_interval_sec: 5
  default_max_minutes: 30
  default_max_rounds: 3
  default_max_files: 10
  default_max_patch_lines: 200
  stop_file: ".selo-stop"
  lock_file: ".selo.lock"
  notify: "stdout"

  # Claims your project must never make. Substring-matched against the diff; any
  # hit fails the safety scan. These are generic examples — replace them with
  # claims specific to your domain.
  forbidden_claims:
    - "PRODUCTION_READY"
    - "BATTLE_TESTED"
    - "FULLY_TESTED"
    - "ENTERPRISE_GRADE"
    - "SOC2_COMPLIANT"
    - "SECURE_BY_DESIGN"

  # runner.mode: "mock" (placeholder echo), "real" (configured command), or
  # "opencode" (discover and run the OpenCode binary). Anything else is refused.
  runner:
    mode: "mock"

  # containment.strategy: only "worktree" is implemented. "docker" and "local"
  # are refused instead of silently running with different isolation.
  containment:
    strategy: "worktree"
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("writing default config: %w", err)
		}
		fmt.Printf("  created config/selo.yaml\n")
	} else {
		fmt.Printf("  config/selo.yaml already exists, skipping\n")
	}

	// Write .gitignore entries
	gitignorePath := filepath.Join(initRepo, ".gitignore")
	gitignoreEntry := "\n# Selo\nqueue/\nruns/\nreceipts/\nworktrees/\n"
	if data, err := os.ReadFile(gitignorePath); err == nil {
		content := string(data)
		if containsStr(content, "queue/") {
			fmt.Printf("  .gitignore already configured\n")
			return nil
		}
		gitignoreEntry = content + gitignoreEntry
	}
	if err := os.WriteFile(gitignorePath, []byte(gitignoreEntry), 0644); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}
	fmt.Printf("  updated .gitignore\n")

	fmt.Printf("\nSelo workspace initialized in %s\n", initRepo)
	fmt.Printf("Next steps:\n")
	fmt.Printf("  1. Review config/selo.yaml\n")
	fmt.Printf("  2. Start the daemon: selo daemon\n")
	fmt.Printf("  3. Submit a task: selo run \"fix the bug\"\n")
	return nil
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

package p45

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const VerdictScopeViolation = "SCOPE_VIOLATION"

func ContractsDir(runsDir, taskID string) string {
	return filepath.Join(runsDir, fmt.Sprintf("run-%s", taskID), "contracts")
}

func ContractsExist(dir string) bool {
	_, err := os.Stat(dir)
	return err == nil
}

func ParseChangedFiles(diff string) []string {
	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				path := strings.TrimPrefix(parts[3], "b/")
				if path != "" && path != "/dev/null" && !seen[path] {
					seen[path] = true
					files = append(files, path)
				}
			}
		}
	}
	return files
}

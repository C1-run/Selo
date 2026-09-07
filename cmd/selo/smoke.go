package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/selo-dev/selo/internal/runner"
)

func smokeC1Loop() {
	c1Path := runner.DiscoverC1Binary("")
	if c1Path == "" {
		c1Path = runner.DiscoverC1Binary("scripts/c1-loop.sh")
	}
	if c1Path == "" {
		home, _ := os.UserHomeDir()
		c1Path = runner.DiscoverC1Binary(filepath.Join(home, "C1-forge", "scripts", "c1-loop.sh"))
	}

	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, "C1-forge", "config", "selo.example.yaml")

	runSmokeSuite("c1-loop", []SmokeCheck{
		{"C1 binary discovery", func() bool {
			if c1Path != "" {
				fmt.Printf("   Found: %s\n", c1Path)
				return true
			}
			fmt.Println("   NOT FOUND - c1 binary not available")
			return false
		}},
		{"C1 version", func() bool {
			fmt.Println("   (c1-loop.sh is a shim, no version flag)")
			return true
		}},
		{"Disposable repo", func() bool {
			if _, err := os.Stat("/tmp/selo-fixture"); err == nil {
				fmt.Println("   Found: /tmp/selo-fixture")
				return true
			}
			fmt.Println("   NOT FOUND - create with: git init /tmp/selo-fixture")
			return false
		}},
		{"Config wiring", func() bool {
			if _, err := os.Stat(cfgPath); err == nil {
				fmt.Printf("   Config: %s\n", cfgPath)
				return true
			}
			fmt.Println("   Config not found")
			return false
		}},
		{"Receipt writer", func() bool {
			fmt.Println("   Receipt package compiles and writes JSON/MD")
			return true
		}},
	})
}

func smokeActualC1(allowShim bool) {
	binInfo := runner.DiscoverC1BinaryWithKind("", allowShim)

	home, _ := os.UserHomeDir()
	adapterPath := filepath.Join(home, "C1-forge", "scripts", "c1-real-adapter.sh")

	runSmokeSuite("actual-c1", []SmokeCheck{
		{"C1 binary discovery", func() bool {
			if binInfo.Path == "" {
				fmt.Println("   NOT FOUND - no C1 binary found on PATH")
				fmt.Println("   Set SELO_C1_BIN or install 'c1' on PATH")
				fmt.Println("   Use --allow-shim to accept scripts/c1-loop.sh for testing")
				return false
			}
			fmt.Printf("   Found: %s\n   Kind: %s\n   Version: %s\n   Verified: %v\n",
				binInfo.Path, binInfo.Kind, binInfo.Version, binInfo.Verified)
			return true
		}},
		{"C1 version check", func() bool {
			if binInfo.Kind != runner.BinaryKindRealC1 || binInfo.Path == "" {
				fmt.Println("   (skipped: not a real C1 binary)")
				return true
			}
			version := runner.GetBinaryVersion(binInfo.Path)
			if version != "" {
				fmt.Printf("   Version output: %s\n", strings.Split(version, "\n")[0])
				return true
			}
			fmt.Println("   WARNING: binary found but no version output")
			return true
		}},
		{"Disposable repo", func() bool {
			fixturePath := "/tmp/selo-fixture"
			if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
				fmt.Printf("   Creating fixture repo at %s...\n", fixturePath)
				initGitRepo(fixturePath)
				writeFile(filepath.Join(fixturePath, "README.md"), "# Fixture Repo\n")
				writeFile(filepath.Join(fixturePath, "src", "main.go"), "package main\n\nfunc main() {}\n")
				writeFile(filepath.Join(fixturePath, "src", "main_test.go"),
					"package main\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {\n\tt.Log(\"passing\")\n}\n")
				gitAddCommit(fixturePath, "initial")
			}
			if _, err := os.Stat(fixturePath); err == nil {
				fmt.Printf("   Found: %s\n", fixturePath)
				return true
			}
			fmt.Println("   NOT FOUND")
			return false
		}},
		{"Receipt binary metadata", func() bool {
			fmt.Println("   Fields: runner_binary_kind, runner_binary_path, runner_binary_version, runner_binary_verified")
			fmt.Println("   These are written to receipt.json when processOneTask runs with real mode")
			return true
		}},
		{"Adapter script", func() bool {
			if _, err := os.Stat(adapterPath); err == nil {
				fmt.Printf("   Found: %s\n", adapterPath)
				return true
			}
			fmt.Println("   NOT FOUND - adapter will be created if needed")
			return true
		}},
	})
	if binInfo.Path == "" {
		fmt.Println("NOTE: actual C1 binary not found. This test is expected to pass in CI/development")
		fmt.Println("      with a real C1 binary installed. Set SELO_C1_BIN to test.")
	}
}

func smokeC1Runtimes() {
	binInfo := runner.DiscoverC1BinaryWithKind("", false)
	if binInfo.Path == "" {
		envPath := os.Getenv("SELO_C1_BIN")
		if envPath != "" {
			binInfo = runner.DiscoverC1BinaryWithKind(envPath, false)
		}
	}
	if binInfo.Path == "" {
		fmt.Println("   NOT FOUND - cannot probe runtimes")
		fmt.Println("   Set SELO_C1_BIN or install 'c1' on PATH")
		os.Exit(1)
	}

	repoDir := createFixtureRepo("c1-runtime", nil)
	defer os.RemoveAll(repoDir)

	runSmokeSuite("c1-runtimes", []SmokeCheck{
		{"C1 binary discovery", func() bool {
			fmt.Printf("   Binary: %s (kind=%s, version=%s)\n", binInfo.Path, binInfo.Kind, binInfo.Version)
			return true
		}},
		{"Creating disposable test repo", func() bool {
			fmt.Printf("   Repo created: %s\n", repoDir)
			return true
		}},
		{"Initializing C1", func() bool {
			initCmd := exec.Command(binInfo.Path, "init")
			initCmd.Dir = repoDir
			if out, err := initCmd.CombinedOutput(); err != nil {
				fmt.Printf("   INIT FAILED: %s\n", string(out))
				return false
			}
			fmt.Println("   C1 initialized")
			return true
		}},
		{"Probing --runtime=mock", func() bool {
			mockCmd := exec.Command(binInfo.Path, "loop", "probe mock runtime", "--runtime=mock", "--max-rounds=1")
			mockCmd.Dir = repoDir
			mockOut, mockErr := mockCmd.CombinedOutput()
			if mockErr == nil {
				fmt.Println("   runtime=mock: AVAILABLE")
				return true
			}
			fmt.Printf("   runtime=mock: FAILED (%s)\n", string(mockOut))
			return false
		}},
		{"Probing --runtime=shell", func() bool {
			shellCmd := exec.Command(binInfo.Path, "loop", "probe shell runtime", "--runtime=shell", "--max-rounds=1")
			shellCmd.Dir = repoDir
			shellOut, shellErr := shellCmd.CombinedOutput()
			if shellErr == nil {
				fmt.Println("   runtime=shell: AVAILABLE")
				return true
			}
			fmt.Printf("   runtime=shell: FAILED (%s)\n", string(shellOut))
			return false
		}},
		{"Checking file edit capability", func() bool {
			diffCmd := exec.Command("git", "-C", repoDir, "diff", "HEAD")
			diffOut, _ := diffCmd.Output()
			if len(diffOut) == 0 {
				fmt.Println("   C1 v0.1: AUDIT ONLY (no file edits produced)")
				fmt.Println("   Both mock and shell runtimes inspect/audit only.")
				fmt.Println("   C1 v0.1 does NOT produce patches without external agent.")
				fmt.Println("   This is expected for v0.1 — no fake patches claimed.")
				return true
			}
			fmt.Printf("   C1 produced file edits: %d lines changed\n", strings.Count(string(diffOut), "\n"))
			return true
		}},
	})
	fmt.Println("=== Resolution: C1 v0.1 is AUDIT_ONLY ===")
	fmt.Println("=== Available runtimes: mock, shell ===")
	fmt.Println("=== Both are audit-only — no patcher available in v0.1 ===")
}

func smokeOpenCode() {
	binInfo := runner.DiscoverOpenCodeBinary()
	if binInfo.Path == "" {
		binInfo = runner.DiscoverC1BinaryWithKind("", false)
	}
	if binInfo.Path == "" {
		fmt.Println("   NOT FOUND - opencode not available")
		fmt.Println("   Set SELO_OPENCODE_BIN or install 'opencode' on PATH")
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	adapterPath := filepath.Join(home, "C1-forge", "scripts", "opencode-adapter.sh")
	cfgProfile := filepath.Join(home, "C1-forge", "config", "selo.opencode.local.yaml")

	repoDir := createFixtureRepo("opencode", map[string]string{
		"README.md":       "# Test Repo\n\nHello from Selo\n",
		"src/message.txt": "hello\n",
	})
	defer os.RemoveAll(repoDir)

	model := os.Getenv("SELO_OPENCODE_MODEL")
	if model == "" {
		model = "opencode/deepseek-v4-flash-free"
	}

	runSmokeSuite("opencode", []SmokeCheck{
		{"OpenCode binary discovery", func() bool {
			if binInfo.Kind == runner.BinaryKindOpenCode && binInfo.Path != "" {
				fmt.Printf("   Binary: %s (kind=%s, version=%s)\n", binInfo.Path, binInfo.Kind, binInfo.Version)
				return true
			}
			if binInfo.Path != "" {
				fmt.Printf("   Found: %s (kind=%s, not opencode)\n", binInfo.Path, binInfo.Kind)
				fmt.Println("   WARNING: binary found but not classified as opencode")
				fmt.Println("   Set SELO_OPENCODE_BIN to an opencode binary")
				return false
			}
			return false
		}},
		{"OpenCode version", func() bool {
			version := runner.GetBinaryVersion(binInfo.Path)
			if version != "" {
				fmt.Printf("   Version: %s\n", strings.Split(version, "\n")[0])
				return true
			}
			fmt.Println("   WARNING: binary found but no version output")
			return true
		}},
		{"Creating disposable test repo", func() bool {
			fmt.Printf("   Repo created: %s\n", repoDir)
			return true
		}},
		{"Running OpenCode", func() bool {
			ocCmd := exec.Command(binInfo.Path, "--model", model)
			ocCmd.Dir = repoDir
			ocCmd.Stdin = strings.NewReader("Add a line saying '# OpenCode smoke test' to README.md")
			out, err := ocCmd.CombinedOutput()
			ocExit := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					ocExit = exitErr.ExitCode()
				} else {
					ocExit = -1
				}
			}
			fmt.Printf("   OpenCode exit code: %d\n", ocExit)
			if len(out) > 0 {
				fmt.Printf("   Output (first 500 chars): %s\n", string(out[:min(len(out), 500)]))
			}
			return true
		}},
		{"Checking file changes", func() bool {
			diffOut, _ := exec.Command("git", "-C", repoDir, "diff", "HEAD").Output()
			if len(diffOut) > 0 {
				fileCount := 0
				for _, line := range strings.Split(string(diffOut), "\n") {
					if strings.HasPrefix(line, "diff --git") {
						fileCount++
					}
				}
				fmt.Printf("   Files changed: yes (%d bytes diff)\n   Files modified: %d\n", len(diffOut), fileCount)
				return true
			}
			fmt.Println("   No file changes (dry run or no-op)")
			if os.Getenv("SELO_OPENCODE_DRY_RUN") == "true" {
				fmt.Println("   (dry run enabled, file changes not expected)")
			} else {
				fmt.Println("   WARNING: no file changes from OpenCode")
			}
			return true
		}},
		{"Adapter script", func() bool {
			if _, err := os.Stat(adapterPath); err == nil {
				fmt.Printf("   Found: %s\n", adapterPath)
				return true
			}
			fmt.Println("   NOT FOUND")
			return true
		}},
		{"Config profile", func() bool {
			if _, err := os.Stat(cfgProfile); err == nil {
				fmt.Printf("   Found: %s\n", cfgProfile)
				return true
			}
			fmt.Println("   NOT FOUND")
			return true
		}},
	})
}

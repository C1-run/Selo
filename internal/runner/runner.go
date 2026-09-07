package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RunnerMode specifies how the C1 Loop is executed.
type RunnerMode string

const (
	ModeReal RunnerMode = "real"
	ModeMock RunnerMode = "mock"
)

// BinaryKind describes the type of C1 binary discovered.
type BinaryKind string

const (
	BinaryKindRealC1   BinaryKind = "real_c1"
	BinaryKindShim     BinaryKind = "shim"
	BinaryKindOpenCode BinaryKind = "opencode"
	BinaryKindUnknown  BinaryKind = "unknown"
)

// RunnerConfig holds the command template for the real runner mode.
type RunnerConfig struct {
	Command string   // e.g. "c1" or "opencode"
	Args    []string // e.g. ["loop", "--task-file", "{{task_file}}", "--workdir", "{{worktree}}"]
}

// C1Result captures the outcome of running a C1 Loop.
type C1Result struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	Duration   time.Duration
	TimedOut   bool
	TestOutput string
	Diff       string
	CommandLog string // the actual command that was run
	Mode       RunnerMode
}

// C1LoopRunner runs the C1 Loop as a subprocess with the given prompt/task.
type C1LoopRunner struct {
	WorkDir    string // working directory (worktree)
	TaskPath   string // path to task.md
	MaxMinutes int
	StopFile   string
	Mode       RunnerMode
	Config     RunnerConfig
	BaseDir    string // C1-forge base directory (for resolving relative command paths)
}

// NewC1LoopRunner creates a new runner with the given mode and config.
// If config is nil or empty, it defaults to mock mode.
// baseDir is the C1-forge base directory, used to resolve relative command paths.
func NewC1LoopRunner(workDir, taskPath string, maxMinutes int, stopFile string, mode RunnerMode, config RunnerConfig) *C1LoopRunner {
	baseDir, _ := os.Getwd()
	return &C1LoopRunner{
		WorkDir:    workDir,
		TaskPath:   taskPath,
		MaxMinutes: maxMinutes,
		StopFile:   stopFile,
		Mode:       mode,
		Config:     config,
		BaseDir:    baseDir,
	}
}

// NewMockRunner creates a runner in mock mode.
func NewMockRunner(workDir, taskPath string, maxMinutes int, stopFile string) *C1LoopRunner {
	return NewC1LoopRunner(workDir, taskPath, maxMinutes, stopFile, ModeMock, RunnerConfig{})
}

// Run executes the C1 Loop with a timeout, dispatching to real or mock mode.
func (r *C1LoopRunner) Run() *C1Result {
	if r.Mode == ModeReal && r.Config.Command != "" {
		return r.runReal()
	}
	return r.runMock()
}

// runReal executes the configured command with template substitution.
func (r *C1LoopRunner) runReal() *C1Result {
	result := &C1Result{Mode: ModeReal}
	start := time.Now()

	// Build command line with template substitution
	subst := func(s string) string {
		s = strings.ReplaceAll(s, "{{task_file}}", r.TaskPath)
		s = strings.ReplaceAll(s, "{{worktree}}", r.WorkDir)
		s = strings.ReplaceAll(s, "{{task_id}}", filepathBase(r.TaskPath))
		s = strings.ReplaceAll(s, "{{max_minutes}}", fmt.Sprintf("%d", r.MaxMinutes))
		return s
	}

	fullArgs := make([]string, len(r.Config.Args))
	for i, arg := range r.Config.Args {
		fullArgs[i] = subst(arg)
	}

	result.CommandLog = fmt.Sprintf("%s %s", r.Config.Command, strings.Join(fullArgs, " "))

	// Resolve relative command paths against the base directory
	cmdPath := r.Config.Command
	if strings.HasPrefix(cmdPath, "./") || strings.HasPrefix(cmdPath, "../") {
		if r.BaseDir != "" {
			cmdPath = filepathAbs(filepathJoin(r.BaseDir, cmdPath))
		}
	}

	cmd := exec.Command(cmdPath, fullArgs...)
	cmd.Dir = r.WorkDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("C1_TASK=%s", r.TaskPath),
		fmt.Sprintf("C1_WORKDIR=%s", r.WorkDir),
		fmt.Sprintf("C1_MAX_MINUTES=%d", r.MaxMinutes),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		result.Stderr = err.Error()
		result.ExitCode = -1
		result.Duration = time.Since(start)
		return result
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	timeout := time.Duration(r.MaxMinutes) * time.Minute
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		timer.Stop()
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		result.Duration = time.Since(start)
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
			}
		} else {
			result.ExitCode = 0
		}

	case <-timer.C:
		cmd.Process.Kill()
		<-done
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		result.Duration = time.Since(start)
		result.TimedOut = true
		result.ExitCode = -1
	}

	return result
}

// runMock executes the placeholder echo command (Phase 0 behavior).
func (r *C1LoopRunner) runMock() *C1Result {
	result := &C1Result{Mode: ModeMock}
	start := time.Now()

	mockCmd := fmt.Sprintf("echo 'C1 Loop mock mode: task=%s workdir=%s'; echo 'No changes made.'", r.TaskPath, r.WorkDir)
	result.CommandLog = mockCmd

	cmd := exec.Command("sh", "-c", mockCmd)
	cmd.Dir = r.WorkDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("C1_TASK=%s", r.TaskPath),
		fmt.Sprintf("C1_WORKDIR=%s", r.WorkDir),
		fmt.Sprintf("C1_MAX_MINUTES=%d", r.MaxMinutes),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		result.Stderr = err.Error()
		result.ExitCode = -1
		result.Duration = time.Since(start)
		return result
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	timeout := time.Duration(r.MaxMinutes) * time.Minute
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		timer.Stop()
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		result.Duration = time.Since(start)
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
			}
		} else {
			result.ExitCode = 0
		}
	case <-timer.C:
		cmd.Process.Kill()
		<-done
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		result.Duration = time.Since(start)
		result.TimedOut = true
		result.ExitCode = -1
	}

	return result
}

// filepathAbs is a simple absolutize helper (no external deps).
func filepathAbs(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path
	}
	wd, _ := os.Getwd()
	if wd != "" {
		return filepathJoin(wd, path)
	}
	return path
}

// filepathJoin joins path elements with "/".
func filepathJoin(elem ...string) string {
	var result string
	for _, e := range elem {
		if result == "" {
			result = strings.TrimRight(e, "/")
		} else {
			result = result + "/" + strings.Trim(e, "/")
		}
	}
	return result
}

// filepathBase returns the base name (without extension) of a file path.
func filepathBase(path string) string {
	// Find last / or \
	idx := strings.LastIndexAny(path, "/\\")
	if idx >= 0 {
		path = path[idx+1:]
	}
	// Remove extension
	if dot := strings.LastIndex(path, "."); dot > 0 {
		path = path[:dot]
	}
	return path
}

// CaptureDiff runs git diff HEAD to capture all changes (staged and unstaged) in the worktree.
func CaptureDiff(workDir string) string {
	// Try git diff HEAD first (catches staged + unstaged)
	cmd := exec.Command("git", "diff", "HEAD", "--no-color")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return string(out)
	}
	// Fall back to git diff (unstaged only) if HEAD diff fails
	cmd2 := exec.Command("git", "diff", "--no-color")
	cmd2.Dir = workDir
	out2, err2 := cmd2.Output()
	if err2 != nil {
		return fmt.Sprintf("(error capturing diff: %v)", err2)
	}
	return string(out2)
}

// CaptureBaseCommit returns the current HEAD commit hash of the worktree.
func CaptureBaseCommit(workDir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CaptureTestOutput runs a test command and captures the result.
func CaptureTestOutput(workDir string, commands []string) string {
	var all bytes.Buffer
	for _, cmdStr := range commands {
		all.WriteString(fmt.Sprintf("$ %s\n", cmdStr))
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			continue
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		all.Write(out)
		if err != nil {
			if cmd.ProcessState != nil {
				all.WriteString(fmt.Sprintf("\n[exit code: %d]\n", cmd.ProcessState.ExitCode()))
			} else {
				all.WriteString(fmt.Sprintf("\n[error: %v]\n", err))
			}
		}
	}
	return all.String()
}

// RunForbiddenClaimsScan checks for forbidden terms in the worktree.
func RunForbiddenClaimsScan(workDir string, forbiddenTerms []string) ([]string, error) {
	var hits []string
	for _, term := range forbiddenTerms {
		cmd := exec.Command("grep", "-rn", "--include=*.go", "--include=*.rs", "--include=*.md", "--include=*.yaml", "--include=*.toml", "--include=*.json", "--include=*.txt", term, workDir)
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				if line != "" {
					hits = append(hits, fmt.Sprintf("%s: %s", term, line))
				}
			}
		}
	}
	return hits, nil
}

// RunSecretScan runs a simple secret pattern scan.
func RunSecretScan(workDir string) ([]string, error) {
	patterns := []string{
		`-----BEGIN\s+(RSA|OPENSSH|EC|DSA|PGP)\s+PRIVATE\s+KEY-----`,
		`(?:api[_-]?key|secret|token|password)\s*[:=]\s*['"][^'"]+['"]`,
	}
	var hits []string
	for _, pattern := range patterns {
		cmd := exec.Command("grep", "-rni", "-E", pattern, workDir)
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				if line != "" {
					hits = append(hits, line)
				}
			}
		}
	}
	return hits, nil
}

// GetChangedFiles extracts repo-relative changed file paths (b/ side)
// from a unified diff. workDir is reserved for a future git fallback.
func GetChangedFiles(workDir, diff string) ([]string, error) {
	var files []string
	seen := map[string]bool{}
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "diff --git") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		p := strings.TrimPrefix(parts[len(parts)-1], "b/")
		if p != "" && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	return files, nil
}

// RunForbiddenClaimsScanDiffScoped scans only changed files for forbidden terms.
// Falls back to a full worktree scan when changedFiles is empty.
func RunForbiddenClaimsScanDiffScoped(workDir string, forbiddenTerms, changedFiles []string) ([]string, error) {
	if len(changedFiles) == 0 {
		return RunForbiddenClaimsScan(workDir, forbiddenTerms)
	}
	var hits []string
	for _, term := range forbiddenTerms {
		args := append([]string{"-rn", term}, changedFiles...)
		cmd := exec.Command("grep", args...)
		cmd.Dir = workDir
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				if line != "" {
					hits = append(hits, fmt.Sprintf("%s: %s", term, line))
				}
			}
		}
	}
	return hits, nil
}

var secretScanPatterns = []string{
	`-----BEGIN\s+(RSA|OPENSSH|EC|DSA|PGP)\s+PRIVATE\s+KEY-----`,
	`(?:api[_-]?key|secret|token|password)\s*[:=]\s*['"][^'"]+['"]`,
}

// RunSecretScanDiffScoped runs secret patterns over changed files only.
// Falls back to a full worktree scan when changedFiles is empty.
func RunSecretScanDiffScoped(workDir string, changedFiles []string) ([]string, error) {
	if len(changedFiles) == 0 {
		return RunSecretScan(workDir)
	}
	var hits []string
	for _, pattern := range secretScanPatterns {
		args := append([]string{"-rni", "-E", pattern}, changedFiles...)
		cmd := exec.Command("grep", args...)
		cmd.Dir = workDir
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				if line != "" {
					hits = append(hits, line)
				}
			}
		}
	}
	return hits, nil
}

// CheckForbiddenFileEdit checks if any files outside allowed_files or inside forbidden_files were modified.
func CheckForbiddenFileEdit(diff, workDir string, allowedFiles, forbiddenFiles []string) (bool, string) {
	if diff == "" {
		return false, ""
	}
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "diff --git") {
			continue
		}
		// Extract the file path (second path after "b/")
		parts := strings.Split(line, " ")
		if len(parts) < 3 {
			continue
		}
		filePath := strings.TrimPrefix(parts[len(parts)-1], "b/")

		// Check if file is in the forbidden list
		for _, forbidden := range forbiddenFiles {
			if strings.Contains(filePath, forbidden) {
				return true, fmt.Sprintf("forbidden file modified: %s (matches: %s)", filePath, forbidden)
			}
		}

		// Check if file is in the allowed list (if allowed list is non-empty)
		if len(allowedFiles) > 0 {
			allowed := false
			for _, allowedPath := range allowedFiles {
				if strings.HasPrefix(filePath, allowedPath) {
					allowed = true
					break
				}
			}
			if !allowed {
				return true, fmt.Sprintf("file outside allowed scope: %s", filePath)
			}
		}
	}
	return false, ""
}

// C1BinaryInfo holds the result of binary discovery.
type C1BinaryInfo struct {
	Path     string     `json:"path"`
	Kind     BinaryKind `json:"kind"`
	Version  string     `json:"version"`
	Verified bool       `json:"verified"`
}

// DiscoverC1Binary finds the C1 Loop binary.
// It checks the explicit path first, then searches PATH.
// Returns the absolute path to the binary, or empty string if not found.
func DiscoverC1Binary(explicitPath string) string {
	info := DiscoverC1BinaryWithKind(explicitPath, false)
	return info.Path
}

// DiscoverC1BinaryWithKind finds the C1 Loop binary and returns metadata.
// If allowShim is true, c1-loop.sh is accepted; otherwise it is rejected.
// Preferred order:
//  1. explicit config runner.command
//  2. SELO_C1_BIN env var
//  3. SELO_OPENCODE_BIN env var
//  4. PATH lookup for c1
//  5. PATH lookup for opencode
//  6. PATH lookup for c1-loop / c1-loop.sh (only if allowShim)
func DiscoverC1BinaryWithKind(explicitPath string, allowShim bool) C1BinaryInfo {
	// 1. Explicit path from config
	if explicitPath != "" {
		if isExecutable(explicitPath) {
			info := C1BinaryInfo{Path: explicitPath, Verified: true}
			info.Kind, info.Version = classifyAndVersion(explicitPath)
			return info
		}
		absPath := filepathAbs(explicitPath)
		if absPath != explicitPath && isExecutable(absPath) {
			info := C1BinaryInfo{Path: absPath, Verified: true}
			info.Kind, info.Version = classifyAndVersion(absPath)
			return info
		}
		return C1BinaryInfo{Kind: BinaryKindUnknown}
	}

	// 2. Environment variable SELO_C1_BIN
	envPath := os.Getenv("SELO_C1_BIN")
	if envPath != "" {
		if isExecutable(envPath) {
			info := C1BinaryInfo{Path: envPath, Verified: true}
			info.Kind, info.Version = classifyAndVersion(envPath)
			return info
		}
		absEnv := filepathAbs(envPath)
		if absEnv != envPath && isExecutable(absEnv) {
			info := C1BinaryInfo{Path: absEnv, Verified: true}
			info.Kind, info.Version = classifyAndVersion(absEnv)
			return info
		}
	}

	// 3. Environment variable SELO_OPENCODE_BIN
	ocEnvPath := os.Getenv("SELO_OPENCODE_BIN")
	if ocEnvPath != "" {
		if isExecutable(ocEnvPath) {
			info := C1BinaryInfo{Path: ocEnvPath, Verified: true}
			info.Kind, info.Version = classifyAndVersion(ocEnvPath)
			return info
		}
		absOC := filepathAbs(ocEnvPath)
		if absOC != ocEnvPath && isExecutable(absOC) {
			info := C1BinaryInfo{Path: absOC, Verified: true}
			info.Kind, info.Version = classifyAndVersion(absOC)
			return info
		}
	}

	// 4-6. PATH lookup
	candidates := []string{"c1", "opencode"}
	if allowShim {
		candidates = append(candidates, "c1-loop", "c1-loop.sh")
	}

	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err == nil && isExecutable(path) {
			info := C1BinaryInfo{Path: path, Verified: true}
			info.Kind, info.Version = classifyAndVersion(path)
			return info
		}
	}

	return C1BinaryInfo{Kind: BinaryKindUnknown}
}

// DiscoverOpenCodeBinary finds the OpenCode binary specifically.
// Checks SELO_OPENCODE_BIN env var first, then PATH for opencode.
func DiscoverOpenCodeBinary() C1BinaryInfo {
	// 1. Environment variable SELO_OPENCODE_BIN
	ocEnvPath := os.Getenv("SELO_OPENCODE_BIN")
	if ocEnvPath != "" {
		if isExecutable(ocEnvPath) {
			info := C1BinaryInfo{Path: ocEnvPath, Verified: true}
			info.Kind, info.Version = classifyAndVersion(ocEnvPath)
			return info
		}
		absOC := filepathAbs(ocEnvPath)
		if absOC != ocEnvPath && isExecutable(absOC) {
			info := C1BinaryInfo{Path: absOC, Verified: true}
			info.Kind, info.Version = classifyAndVersion(absOC)
			return info
		}
	}

	// 2. PATH lookup for opencode
	path, err := exec.LookPath("opencode")
	if err == nil && isExecutable(path) {
		info := C1BinaryInfo{Path: path, Verified: true}
		info.Kind, info.Version = classifyAndVersion(path)
		return info
	}

	return C1BinaryInfo{Kind: BinaryKindUnknown}
}

// GetBinaryVersion runs the binary's version command and returns the output.
func GetBinaryVersion(binPath string) string {
	if binPath == "" {
		return ""
	}
	// Try --version first, then version subcommand
	for _, arg := range []string{"--version", "version"} {
		cmd := exec.Command(binPath, arg)
		out, err := cmd.Output()
		if err == nil {
			v := strings.TrimSpace(string(out))
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// classifyAndVersion determines the binary kind and version.
func classifyAndVersion(binPath string) (BinaryKind, string) {
	baseName := filepathBase(binPath)
	kind := BinaryKindUnknown
	switch {
	case baseName == "c1":
		kind = BinaryKindRealC1
	case baseName == "opencode":
		kind = BinaryKindOpenCode
	case baseName == "c1-loop" || baseName == "c1-loop.sh":
		kind = BinaryKindShim
	case strings.HasPrefix(baseName, "c1_"):
		kind = BinaryKindRealC1
	}
	version := GetBinaryVersion(binPath)
	return kind, version
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Check executable bit and not a directory
	return !info.IsDir() && (info.Mode().Perm()&0111 != 0)
}

// MapVerdict determines the appropriate verdict based on result, safety hits, and limits.
// This is the canonical verdict mapping for Phase 0.1+.
func MapVerdict(result *C1Result, safetyHits []string, limitViolation bool) string {
	// Safety violations take precedence
	if len(safetyHits) > 0 {
		for _, hit := range safetyHits {
			if strings.Contains(hit, "forbidden") || strings.Contains(hit, "secret") {
				return "FAILED_SAFETY"
			}
			if strings.Contains(hit, "exceeded") || strings.Contains(hit, "too many") {
				return "FAILED_LIMIT_EXCEEDED"
			}
		}
		// Non-critical safety hits → needs human review
		return "NEEDS_HUMAN"
	}

	if result.TimedOut {
		return "FAILED_TIMEOUT"
	}

	if limitViolation {
		return "FAILED_LIMIT_EXCEEDED"
	}

	// Non-zero exit with useful diff = partial failure
	if result.ExitCode != 0 {
		// If there's a meaningful diff, it's a partial failure (did work but tests fail)
		if len(result.Diff) > 50 {
			return "PARTIAL_FAILURE"
		}
		// No diff and exit non-zero = something went wrong
		// Check if there was some test output indicating failure
		if len(result.TestOutput) > 0 {
			return "PARTIAL_FAILURE"
		}
		// Otherwise it's a plain failure but not timeout/safety
		return "PARTIAL_FAILURE"
	}

	// Exit 0: check test output for failures
	if strings.Contains(result.TestOutput, "FAIL") || strings.Contains(result.TestOutput, "failed") {
		if len(result.Diff) > 50 {
			return "PARTIAL_FAILURE"
		}
		return "PARTIAL_FAILURE"
	}

	// Exit 0: check if anything actually happened
	if len(result.Diff) == 0 || len(result.Diff) < 50 {
		return "NOOP_WITH_RECEIPT"
	}

	return "SUCCESS_WITH_RECEIPT"
}

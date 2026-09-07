package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/selo-dev/selo/internal/auditlog"
	"github.com/selo-dev/selo/internal/governor"
	"github.com/selo-dev/selo/internal/soak"
	"github.com/selo-dev/selo/internal/supply"
	"github.com/selo-dev/selo/internal/containment"
	"github.com/selo-dev/selo/internal/gatechain"
	"github.com/selo-dev/selo/internal/notify"
	"github.com/selo-dev/selo/internal/opencode"
	"github.com/selo-dev/selo/internal/p45"
	"github.com/selo-dev/selo/internal/pinocchio"
	"github.com/selo-dev/selo/internal/queue"
	"github.com/selo-dev/selo/internal/receipt"
	"github.com/selo-dev/selo/internal/runner"
	"github.com/selo-dev/selo/internal/testintegrity"
	"github.com/selo-dev/selo/internal/workspace"
)

// RunnerConfig holds the command configuration for the C1 Loop runner.
type RunnerConfig struct {
	Mode    string   `yaml:"mode"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// Config holds the daemon configuration.
type Config struct {
	Forge struct {
		PollIntervalSec     int           `yaml:"poll_interval_sec"`
		DefaultMaxMinutes   int           `yaml:"default_max_minutes"`
		DefaultMaxRounds    int           `yaml:"default_max_rounds"`
		DefaultMaxFiles     int           `yaml:"default_max_files"`
		DefaultMaxPatchLines int          `yaml:"default_max_patch_lines"`
		StopFile            string        `yaml:"stop_file"`
		LockFile            string        `yaml:"lock_file"`
		Notify              string        `yaml:"notify"`         // legacy: "stdout" | "ntfy"
		NtfyTopic           string        `yaml:"ntfy_topic"`     // legacy: ntfy topic
		NotifyConfig        notify.Config `yaml:"notify_config"`  // new: structured config
		ScanTools           struct {
			SecretScan       string `yaml:"secret_scan"`
			ForbiddenClaimsScan string `yaml:"forbidden_claims_scan"`
			Pinocchio        string `yaml:"pinocchio"`
		} `yaml:"scan_tools"`
		ForbiddenClaims []string     `yaml:"forbidden_claims"`
		Runner         RunnerConfig `yaml:"runner"`
		OpenCode       struct {
			Model                   string   `yaml:"model"`
			Agent                   string   `yaml:"agent"`
			ServeTimeout            int      `yaml:"serve_timeout"`
			DangerouslySkipPermissions bool  `yaml:"dangerously_skip_permissions"`
			PermissionAllowlist     []string `yaml:"permission_allowlist"`
		} `yaml:"opencode"`
		Containment struct {
			Strategy string `yaml:"strategy"` // worktree | docker | local
			Image    string `yaml:"image"`    // docker image
		} `yaml:"containment"`
	} `yaml:"forge"`
}

func main() {
	Execute()
}

func processOneTask(qm *queue.QueueManager, rw *receipt.ReceiptWriter, wtm *workspace.WorktreeManager, cfg *Config) bool {
	// 1. Scan pending tasks
	tasks, err := qm.ScanPending()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[selo] Error scanning pending: %v\n", err)
		return false
	}
	if len(tasks) == 0 {
		return false
	}

	taskPath := tasks[0]
	fmt.Printf("[selo] Claiming task: %s\n", taskPath)

	// 2. Claim task (atomic rename to running)
	runningPath, err := qm.ClaimTask(taskPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[selo] Error claiming task: %v\n", err)
		return false
	}

	// 3. Parse task metadata
	taskMeta, err := receipt.ParseTaskMeta(runningPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[selo] Error parsing task: %v\n", err)
		return false
	}

	taskID := taskMeta.ID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", time.Now().Unix())
	}

	// 4. Create lock file for this task
	taskLockPath := filepath.Join(qm.LockDir(), fmt.Sprintf("%s.lock", taskID))
	taskLock, err := queue.WriteLock(taskLockPath, taskID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[selo] Error writing task lock: %v\n", err)
		// Move to failed
		qm.MoveTask(runningPath, qm.FailedDir())
		return true
	}
	defer taskLock.Remove()

	// 5. Resolve configuration
	maxMinutes := taskMeta.MaxMinutes
	if maxMinutes <= 0 {
		maxMinutes = cfg.Forge.DefaultMaxMinutes
	}
	maxRounds := taskMeta.MaxRounds
	if maxRounds <= 0 {
		maxRounds = cfg.Forge.DefaultMaxRounds
	}

	// 6. Parse additional governor config from task
	_, _, maxFiles, maxPatchLines, commands, allowedFiles, forbiddenFiles, forbiddenClaims, allowTestMods, _ := governor.ParseTaskConfig(runningPath)
	if maxFiles <= 0 {
		maxFiles = cfg.Forge.DefaultMaxFiles
	}
	if maxPatchLines <= 0 {
		maxPatchLines = cfg.Forge.DefaultMaxPatchLines
	}
	if len(forbiddenClaims) == 0 {
		forbiddenClaims = cfg.Forge.ForbiddenClaims
	}

	gov := governor.NewGovernor(maxRounds, maxFiles, maxPatchLines, filepath.Join(qm.BaseDir, cfg.Forge.StopFile))

	// 6b. Resolve runner mode (early so failure receipts can report the truth)
	runnerMode := runner.ModeMock
	runnerCfg := runner.RunnerConfig{}
	binInfo := runner.C1BinaryInfo{Kind: runner.BinaryKindUnknown}
	if cfg.Forge.Runner.Mode == "real" && cfg.Forge.Runner.Command != "" {
		runnerMode = runner.ModeReal
		runnerCfg = runner.RunnerConfig{
			Command: cfg.Forge.Runner.Command,
			Args:    cfg.Forge.Runner.Args,
		}
		// Discover binary metadata for the configured command
		binInfo = runner.DiscoverC1BinaryWithKind(cfg.Forge.Runner.Command, false)
	} else if cfg.Forge.Runner.Mode == "real" && cfg.Forge.Runner.Command == "" {
		// Try env var and PATH without explicit config command
		binInfo = runner.DiscoverC1BinaryWithKind("", false)
		if binInfo.Path != "" {
			runnerMode = runner.ModeReal
			runnerCfg = runner.RunnerConfig{
				Command: binInfo.Path,
				Args:    []string{"loop", "--task-file", "{{task_file}}", "--workdir", "{{worktree}}"},
			}
		}
	}

	// 7. Create git worktree
	repoPath := taskMeta.Repo
	if repoPath == "" {
		repoPath = qm.BaseDir
	}

	worktreePath, err := wtm.CreateWorktree(repoPath, taskID)
	if err != nil {
		// Write failure receipt
		writeFailReceipt(rw, taskMeta, taskID, fmt.Sprintf("worktree creation failed: %v", err), receipt.VerdictInternalError, runnerMode)
		qm.MoveTask(runningPath, qm.FailedDir())
		return true
	}
	defer wtm.RemoveWorktree(repoPath, taskID)

	// 9a. P45: Load contracts if present
	var p45Enabled bool
	var p45Goal *p45.P45GoalContract
	var p45Scope *containment.ScopeContract
	p45ContractsDir := p45.ContractsDir(qm.RunsDir(), taskID)
	if p45.ContractsExist(p45ContractsDir) {
		var loadErr error
		p45Goal, loadErr = p45.LoadGoal(p45ContractsDir)
		if loadErr != nil {
			writeFailReceipt(rw, taskMeta, taskID, fmt.Sprintf("P45 contract load error: %v", loadErr), receipt.VerdictInternalError, runnerMode)
			qm.MoveTask(runningPath, qm.FailedDir())
			return true
		}
		p45Scope, loadErr = p45.LoadScope(p45ContractsDir)
		if loadErr != nil {
			writeFailReceipt(rw, taskMeta, taskID, fmt.Sprintf("P45 contract load error: %v", loadErr), receipt.VerdictInternalError, runnerMode)
			qm.MoveTask(runningPath, qm.FailedDir())
			return true
		}
		_, loadErr = p45.LoadRun(p45ContractsDir)
		if loadErr != nil {
			writeFailReceipt(rw, taskMeta, taskID, fmt.Sprintf("P45 contract load error: %v", loadErr), receipt.VerdictInternalError, runnerMode)
			qm.MoveTask(runningPath, qm.FailedDir())
			return true
		}
		p45Enabled = true
	}

	// 9a-2. Create auditlog adapter if P45 is enabled
	var auditLogAdapter *auditlog.Adapter
	if p45Enabled {
		auditLogAdapter = auditlog.NewAdapter(filepath.Join(qm.RunsDir(), fmt.Sprintf("run-%s", taskID)), taskID, true)
	}

	// 9b. Capture test inventory BEFORE runner execution
	testInvBefore := testintegrity.CaptureInventory(worktreePath, commands)

	// 9. Run the C1 Loop
	// If binary kind is OpenCode, try native adapter first
	absRunningPath, _ := filepath.Abs(runningPath)
	startTime := time.Now()
	var result *runner.C1Result
	opencodeModel := ""
	opencodeAgent := ""
	opencodeTimedOut := false
	opencodeRunInfoPath := ""

	if binInfo.Kind == runner.BinaryKindOpenCode && runnerMode == runner.ModeReal {
		ocAdapter := opencode.NewAdapter(opencode.AdapterOpts{
			BinaryPath:      binInfo.Path,
			WorkDir:         worktreePath,
			TaskFilePath:    absRunningPath,
			Model:           cfg.Forge.OpenCode.Model,
			Agent:           cfg.Forge.OpenCode.Agent,
			MaxMinutes:      maxMinutes,
			ServeTimeoutSec: cfg.Forge.OpenCode.ServeTimeout,
			Permissions:     cfg.Forge.OpenCode.DangerouslySkipPermissions,
			Allowlist:       cfg.Forge.OpenCode.PermissionAllowlist,
		})
		ocResult := ocAdapter.Run(context.Background(), taskMeta.Goal)
		result = &runner.C1Result{
			ExitCode: ocResult.ExitCode,
			TimedOut: ocResult.TimedOut,
			Duration: time.Duration(ocResult.DurationMs) * time.Millisecond,
		}
		opencodeModel = cfg.Forge.OpenCode.Model
		opencodeAgent = cfg.Forge.OpenCode.Agent
		opencodeTimedOut = ocResult.TimedOut
	} else {
		// Fallback: use C1LoopRunner (shell adapter or C1 Loop)
		c1Runner := runner.NewC1LoopRunner(worktreePath, absRunningPath, maxMinutes,
			filepath.Join(qm.BaseDir, cfg.Forge.StopFile), runnerMode, runnerCfg)
		result = c1Runner.Run()
	}
	finishTime := time.Now()

	// 9b. Post-run binary discovery: if configured command is adapter (unknown kind),
	// try to discover the underlying real binary via env/PATH for accurate metadata.
	// Also check for OpenCode run-info from the adapter.
	if binInfo.Kind == runner.BinaryKindUnknown || binInfo.Kind == "" {
		// Check SELO_C1_BIN env var first
		envPath := os.Getenv("SELO_C1_BIN")
		if envPath != "" {
			realBin := runner.DiscoverC1BinaryWithKind(envPath, false)
			if realBin.Path != "" && realBin.Kind != runner.BinaryKindUnknown {
				binInfo = realBin
			}
		}
		// Check SELO_OPENCODE_BIN env var
		if binInfo.Kind == runner.BinaryKindUnknown || binInfo.Kind == "" {
			ocBin := runner.DiscoverOpenCodeBinary()
			if ocBin.Path != "" && ocBin.Kind != runner.BinaryKindUnknown {
				binInfo = ocBin
			}
		}
		// Fallback to PATH discovery
		if binInfo.Kind == runner.BinaryKindUnknown || binInfo.Kind == "" {
			realBin := runner.DiscoverC1BinaryWithKind("", false)
			if realBin.Path != "" && realBin.Kind != runner.BinaryKindUnknown {
				binInfo = realBin
			}
		}
	}

	// Try to read OpenCode run-info from worktree
	ocInfoPath := filepath.Join(worktreePath, "opencode-run-info.json")
	if data, err := os.ReadFile(ocInfoPath); err == nil {
		var oi struct {
			BinaryPath      string `json:"binary_path"`
			ExitCode        int    `json:"exit_code"`
			Model           string `json:"model"`
			Agent           string `json:"agent"`
			ServerPID       int    `json:"server_pid"`
			ServerStarted   string `json:"server_started"`
			ServerKilled    string `json:"server_killed"`
			ServerExitStatus *int   `json:"server_exit_status"`
			RunExitStatus   *int   `json:"run_exit_status"`
			TimeoutHit      bool   `json:"timeout_hit"`
		}
		if json.Unmarshal(data, &oi) == nil {
			opencodeModel = oi.Model
			opencodeAgent = oi.Agent
			opencodeTimedOut = oi.TimeoutHit
			opencodeRunInfoPath = ocInfoPath
			// If binary kind is still unknown, try to classify from the OpenCode binary path
			if binInfo.Kind == runner.BinaryKindUnknown || binInfo.Kind == "" {
				if oi.BinaryPath != "" {
					ocInfo := runner.DiscoverC1BinaryWithKind(oi.BinaryPath, false)
					if ocInfo.Path != "" && ocInfo.Kind != runner.BinaryKindUnknown {
						binInfo = ocInfo
					}
				}
			}
		}
	}

	// Copy opencode-run-info.json to runs dir for artifact preservation (worktree is ephemeral)
	opencodeRunInfoDest := ""
	if opencodeRunInfoPath != "" {
		destDir := filepath.Join(qm.RunsDir(), fmt.Sprintf("run-%s", taskID))
		opencodeRunInfoDest = filepath.Join(destDir, "opencode-run-info.json")
		if data, err := os.ReadFile(opencodeRunInfoPath); err == nil {
			os.MkdirAll(destDir, 0755)
			os.WriteFile(opencodeRunInfoDest, data, 0644)
		}
	}
	// Update the receipt path to point to the preserved copy
	if opencodeRunInfoDest != "" {
		opencodeRunInfoPath = opencodeRunInfoDest
	}

	// 10. Capture base commit, diff, and test output
	baseCommit := runner.CaptureBaseCommit(worktreePath)
	diff := runner.CaptureDiff(worktreePath)
	result.Diff = diff

	// 10a. P45 scope check after diff capture
	if p45Enabled {
		changedFiles := p45.ParseChangedFiles(diff)
		p45Scope.ChangedPaths = changedFiles
		ok, violations := containment.VerifyScopeContract(p45Scope)
		if !ok {
			runDir := filepath.Join(qm.RunsDir(), fmt.Sprintf("run-%s", taskID))
			os.MkdirAll(runDir, 0755)
			slipPath := filepath.Join(runDir, "P45_SLIP.md")
			f, err := os.Create(slipPath)
			if err == nil {
				p45.WriteSlip(f, &p45.P45Slip{
					RunID:               taskID,
					Goal:                p45Goal.Goal,
					Scope:               fmt.Sprintf("%d allowed, %d forbidden", len(p45Scope.AllowedPaths), len(p45Scope.ForbiddenPaths)),
					PatchBudget:         "",
					ChangedFiles:        changedFiles,
					Violation:           violations[0],
					Verdict:             p45.VerdictScopeViolation,
					Reason:              violations[0],
					NextAllowedAction:   "review receipt, inspect changes, decide next step",
					ForbiddenNextAction: "do not start a new run without reviewing this receipt",
				})
				f.Close()
			}
			// Write audit event for P45 stop
			if auditLogAdapter != nil {
				auditLogAdapter.WriteP45StopEvent(taskID, violations[0])
			}
			qm.MoveTask(runningPath, qm.FailedDir())
			return true
		}
	}

	testOutput := ""
	testsPassed := 0
	if len(commands) > 0 {
		testOutput = runner.CaptureTestOutput(worktreePath, commands)
		// Count passing tests: look for "PASS" or "ok" indicators
		for _, line := range strings.Split(testOutput, "\n") {
			if strings.Contains(line, "PASS") || strings.HasPrefix(line, "ok ") {
				testsPassed++
			}
		}
	}

	// 11. Safety scans + Supply chain
	var safetyHits []string
	result.TestOutput = testOutput

	// 11-supply. Supply chain BOM (MCP/Skills) — best-effort, OSV query
	supplyRes, _ := supply.Check(worktreePath)
	var supplyComponents []string
	for _, c := range supplyRes.Components {
		supplyComponents = append(supplyComponents, fmt.Sprintf("%s@%s (%s:%s)", c.Name, c.Version, c.Ecosystem, c.Source))
	}
	if len(supplyRes.Hits) > 0 {
		safetyHits = append(safetyHits, supplyRes.Hits...)
	}

	// 11a. Forbidden file edit check
	violation, msg := runner.CheckForbiddenFileEdit(diff, worktreePath, allowedFiles, forbiddenFiles)
	if violation {
		safetyHits = append(safetyHits, msg)
	}

	// 11b. Forbidden claims scan — diff-scoped (only changed files)
	changedFiles, _ := runner.GetChangedFiles(worktreePath, diff)
	claimsHits, _ := runner.RunForbiddenClaimsScanDiffScoped(worktreePath, forbiddenClaims, changedFiles)
	safetyHits = append(safetyHits, claimsHits...)

	// 11c. Secret scan — diff-scoped
	secretHits, _ := runner.RunSecretScanDiffScoped(worktreePath, changedFiles)
	safetyHits = append(safetyHits, secretHits...)

	// 11d. Patch limit check
	limitViolation, limitMsg := gov.CheckPatchLimits(diff)
	if limitViolation {
		safetyHits = append(safetyHits, limitMsg)
	}

	// 11e. Round limit check
	if gov.CheckRoundsExceeded(0) {
		safetyHits = append(safetyHits, "max rounds exceeded")
	}

	// 11f. Test integrity gate
	testInvAfter := testintegrity.CaptureInventory(worktreePath, commands)
	tiResult := testintegrity.Analyze(testInvBefore, testInvAfter, diff, allowTestMods)

	// Write test integrity artifact
	tiResultPath := filepath.Join(qm.RunsDir(), fmt.Sprintf("run-%s", taskID), "test_integrity.json")
	if tiData, err := json.MarshalIndent(tiResult, "", "  "); err == nil {
		os.MkdirAll(filepath.Dir(tiResultPath), 0755)
		os.WriteFile(tiResultPath, tiData, 0644)
	}

	if !tiResult.Passed {
		safetyHits = append(safetyHits, fmt.Sprintf("test integrity violation: %d tests removed, %d tests modified, %d commands changed",
			len(tiResult.TestsRemoved), len(tiResult.TestsModified), len(tiResult.TestCommandsChanged)))
	}

	// 12. Determine verdict using canonical mapper
	scansPassed := len(safetyHits) == 0
	initialVerdict := runner.MapVerdict(result, safetyHits, limitViolation)

	// Test-only override: if SELO_TEST_INITIAL_VERDICT is set, force initial verdict
	// This allows E2E tests to verify Pinocchio override behavior directly
	// without MapVerdict interfering.
	if forcedVerdict := os.Getenv("SELO_TEST_INITIAL_VERDICT"); forcedVerdict != "" {
		initialVerdict = forcedVerdict
	}

	// 12b. Check for c1 loop receipt and runtime info
	c1LoopReceiptPath := ""
	c1RuntimeRequested := os.Getenv("SELO_C1_RUNTIME")
	if c1RuntimeRequested == "" {
		c1RuntimeRequested = "mock"
	}
	c1RuntimeUsed := ""
	c1RuntimeIsMock := true
	if runnerMode == runner.ModeReal {
		candidate := filepath.Join(worktreePath, "c1-receipt.json")
		if _, err := os.Stat(candidate); err == nil {
			c1LoopReceiptPath = candidate
		}
		// Read runtime info from worktree
		runtimeInfoPath := filepath.Join(worktreePath, "c1-runtime-info.json")
		if data, err := os.ReadFile(runtimeInfoPath); err == nil {
			var ri struct {
				RuntimeRequested string `json:"runtime_requested"`
				RuntimeUsed      string `json:"runtime_used"`
				IsMock           bool   `json:"is_mock"`
			}
			if json.Unmarshal(data, &ri) == nil {
				c1RuntimeUsed = ri.RuntimeUsed
				c1RuntimeIsMock = ri.IsMock
			}
		}
	}
	if c1RuntimeUsed == "" {
		c1RuntimeUsed = c1RuntimeRequested
	}

	// 12c. Run Pinocchio consistency verification
	pinoArtifacts := &pinocchio.VerificationArtifacts{
		TaskMeta:          taskMeta,
		Result:            result,
		SafetyHits:        safetyHits,
		Diff:              diff,
		TestOutput:        testOutput,
		WorktreePath:      worktreePath,
		RunnerMode:        runnerMode,
		C1LoopReceiptPath: c1LoopReceiptPath,
		InitialVerdict:    initialVerdict,
	}
	pinoResult := pinocchio.VerifyForgeReceipt(pinoArtifacts)
	pinoPath := pinocchio.WritePinocchioArtifact(qm.RunsDir(), taskID, pinoResult)

	// 12d. Apply verdict override if Pinocchio recommends a stricter verdict
	finalVerdict := initialVerdict
	verdictOverridden := false
	overrideReason := ""
	if pinoResult.FinalVerdict != initialVerdict {
		// Only override if Pinocchio's final verdict is more severe
		if pinocchioVerdictSeverity(pinoResult.FinalVerdict) > pinocchioVerdictSeverity(initialVerdict) {
			finalVerdict = pinoResult.FinalVerdict
			verdictOverridden = true
			overrideReason = fmt.Sprintf("Pinocchio consistency gate: %s", formatOverrideReason(pinoResult))
		}
	}

	// 12e. Apply test integrity gate override
	tiVerdictSeverity := 0
	if !tiResult.Passed || tiResult.RecommendedVerdict != "" {
		tiVerdictSeverity = pinocchioVerdictSeverity(tiResult.RecommendedVerdict)
	}
	if tiVerdictSeverity > pinocchioVerdictSeverity(finalVerdict) {
		finalVerdict = tiResult.RecommendedVerdict
		verdictOverridden = true
		if overrideReason != "" {
			overrideReason += "; "
		}
		overrideReason += fmt.Sprintf("test integrity gate: %s tests removed, %s tests modified, %s commands changed",
			strings.Join(tiResult.TestsRemoved, ","),
			strings.Join(tiResult.TestsModified, ","),
			strings.Join(tiResult.TestCommandsChanged, ","))
	}

	// 13. Write receipt with all new fields
	forgeReceipt := &receipt.ForgeReceipt{
		ReceiptID:              receipt.GenerateReceiptID(),
		TaskID:                 taskID,
		Verdict:                finalVerdict,
		StartedAt:              startTime,
		FinishedAt:             finishTime,
		DurationSec:            int64(finishTime.Sub(startTime).Seconds()),
		RoundsUsed:             1,
		ExitCode:               result.ExitCode,
		TimedOut:               result.TimedOut,
		RunnerMode:             string(runnerMode),
		RunnerCommand:          result.CommandLog,
		RunnerExitCode:         result.ExitCode,
		RunnerBinaryKind:       string(binInfo.Kind),
		RunnerBinaryPath:       binInfo.Path,
		RunnerBinaryVersion:    binInfo.Version,
		RunnerBinaryVerified:   binInfo.Verified,
		C1RuntimeRequested:     c1RuntimeRequested,
		C1RuntimeUsed:          c1RuntimeUsed,
		C1RuntimeIsMock:        c1RuntimeIsMock,
		OpenCodeModel:          opencodeModel,
		OpenCodeAgent:          opencodeAgent,
		WorktreePath:           worktreePath,
		BaseCommit:             baseCommit,
		TestsPassed:            testsPassed,
		ScansPassed:            scansPassed,
		C1LoopReceiptPath:      c1LoopReceiptPath,
		PinocchioVerified:      pinoResult.Verified,
		PinocchioResultPath:    pinoPath,
		InitialVerdict:         initialVerdict,
		FinalVerdict:           finalVerdict,
		VerdictOverridden:      verdictOverridden,
		OverrideReason:         overrideReason,
		PinocchioFalseClaims:      pinoResult.FalseClaims,
		PinocchioInconsistencies:  pinoResult.Inconsistencies,
		TestIntegrityPassed:       tiResult.Passed,
		TestIntegrityResultPath:   tiResultPath,
		TestsRemoved:             tiResult.TestsRemoved,
		TestsModified:            tiResult.TestsModified,
		TestCommandsChanged:      tiResult.TestCommandsChanged,
		TestInventoryBeforeCount: tiResult.InventoryBeforeCount,
		TestInventoryAfterCount:  tiResult.InventoryAfterCount,
		OpenCodeRunInfoPath:      opencodeRunInfoPath,
		OpenCodeTimedOut:         opencodeTimedOut,
		SupplyComponents:  supplyComponents,
		SupplyHits:        supplyRes.Hits,
		SupplyCheckedAt:   func() *time.Time { t := supplyRes.CheckedAt; return &t }(),
	}

	// Set notification mode on receipt
	forgeReceipt.NotificationMode = resolvedNotifyMode(cfg)

	// 12f. GateChain: adapt receipt into ChainSteps and write steps.json
	runDir := filepath.Join(qm.RunsDir(), fmt.Sprintf("run-%s", taskID))
	if steps, err := gatechain.AdaptReceipt(forgeReceipt); err == nil {
		if data, jsonErr := json.MarshalIndent(steps, "", "  "); jsonErr == nil {
			os.MkdirAll(runDir, 0755)
			os.WriteFile(filepath.Join(runDir, "steps.json"), data, 0644)
		}
		if verdict, verr := gatechain.VerifyAdaptation(forgeReceipt); verr == nil {
			forgeReceipt.GateChainAction = string(verdict.Action)
			forgeReceipt.GateChainStopRequired = verdict.StopRequired
			forgeReceipt.GateChainReviewRequired = verdict.ReviewRequired
			forgeReceipt.GateChainFinalStatus = string(verdict.FinalStatus)
			forgeReceipt.GateChainActionReason = verdict.ActionReason
		}
	}

	rw.WriteReceipt(forgeReceipt, taskMeta, diff, testOutput, safetyHits)

	// 14. Move task to appropriate directory
	targetDir := qm.DoneDir()
	var reviewPath string
	if finalVerdict == receipt.VerdictSafety || finalVerdict == receipt.VerdictTimedOut ||
		finalVerdict == receipt.VerdictInternalError || finalVerdict == receipt.VerdictLimitExceeded {
		targetDir = qm.FailedDir()
	} else if finalVerdict == receipt.VerdictNeedsHuman || finalVerdict == receipt.VerdictPartial {
		qm.MoveTask(runningPath, qm.ReviewDir())
		reviewPath = filepath.Join(qm.ReviewDir(), fmt.Sprintf("task-%s.md", taskID))
		rw.WriteReviewMD(reviewPath, taskID, diff, testOutput, safetyHits, finalVerdict,
			pinoResult.RecommendedVerdict, pinoResult.Inconsistencies,
			tiResult.Passed, tiResult.TestsRemoved, tiResult.TestsModified, tiResult.TestCommandsChanged)
	} else {
		qm.MoveTask(runningPath, targetDir)
		reviewPath = filepath.Join(targetDir, fmt.Sprintf("task-%s.md", taskID))
		rw.WriteReviewMD(reviewPath, taskID, diff, testOutput, safetyHits, finalVerdict,
			pinoResult.RecommendedVerdict, pinoResult.Inconsistencies,
			tiResult.Passed, tiResult.TestsRemoved, tiResult.TestsModified, tiResult.TestCommandsChanged)
	}

	// 15. Send notification after all artifacts exist
	notifResult := sendNotification(context.Background(), cfg, forgeReceipt, taskMeta, reviewPath, qm.RunsDir())
	forgeReceipt.NotificationSuccess = notifResult.success
	forgeReceipt.NotificationError = notifResult.errMsg

	// Update receipt JSON with notification outcome
	updateReceiptNotification(qm.RunsDir(), taskID, forgeReceipt)

	// 15b. Anchor final receipt to receipt-anchor branch (best-effort, skip if SELO_DISABLE_ANCHOR=1)
	if os.Getenv("SELO_DISABLE_ANCHOR") != "1" {
		receiptPathFinal := filepath.Join(qm.RunsDir(), fmt.Sprintf("run-%s", taskID), "receipt.json")
		if anchorRes, err := receipt.AnchorReceipt(receiptPathFinal, qm.BaseDir); err == nil {
		forgeReceipt.AnchorCommit = anchorRes.GitCommit
		forgeReceipt.AnchorBranch = anchorRes.GitBranch
		t := anchorRes.AnchoredAt
		forgeReceipt.AnchoredAt = &t
		// Update receipt with anchor info (anchor excluded from canonical, no re-sign)
		if data, err := json.MarshalIndent(forgeReceipt, "", "  "); err == nil {
			os.WriteFile(receiptPathFinal, data, 0644)
			receiptsDir := filepath.Join(filepath.Dir(qm.RunsDir()), "receipts")
			if entries, err := os.ReadDir(receiptsDir); err == nil {
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), forgeReceipt.ReceiptID) && strings.HasSuffix(e.Name(), ".json") {
						os.WriteFile(filepath.Join(receiptsDir, e.Name()), data, 0644)
						break
					}
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "[selo] anchor failed (non-fatal): %v\n", err)
	}
	}

	return true
}

func writeFailReceipt(rw *receipt.ReceiptWriter, taskMeta *receipt.TaskMeta, taskID, errMsg, verdict string, runnerMode runner.RunnerMode) {
	now := time.Now()
	failRec := &receipt.ForgeReceipt{
		ReceiptID:      receipt.GenerateReceiptID(),
		TaskID:         taskID,
		Verdict:        verdict,
		StartedAt:      now,
		FinishedAt:     now,
		DurationSec:    0,
		Error:          errMsg,
		RunnerMode:     string(runnerMode),
		ScansPassed:    false,
		RunnerExitCode: -1,
	}
	rw.WriteReceipt(failRec, taskMeta, "", "", nil)
}

// runOpenCodeSoak runs a bounded soak with real OpenCode against disposable fixture repos.
func runOpenCodeSoak(cfg soak.SoakConfig, ocBin, ocModel string) {
	fmt.Printf("[selo] Starting OpenCode soak: tasks=%d, duration=%v, out=%s\n",
		cfg.TaskCount, cfg.Duration, cfg.OutDir)
	fmt.Printf("[selo] OpenCode binary: %s\n", ocBin)
	fmt.Printf("[selo] OpenCode model: %s\n", ocModel)

	os.MkdirAll(cfg.OutDir, 0755)

	// Create fixture base directory for task repos
	fixtureBase := filepath.Join(cfg.OutDir, "fixtures")
	os.RemoveAll(fixtureBase)
	os.MkdirAll(fixtureBase, 0755)

	// Generate OpenCode fixture repos
	fixtureRepos := generateOpenCodeFixtures(fixtureBase)

	// Generate tasks
	tasks := generateOpenCodeTasks(fixtureRepos, cfg.TaskCount)

	// Open heartbeat file
	heartbeatPath := filepath.Join(cfg.OutDir, "heartbeat.jsonl")
	heartbeatFile, err := os.Create(heartbeatPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[selo] Error creating heartbeat: %v\n", err)
		os.Exit(1)
	}
	defer heartbeatFile.Close()

	// Set env vars for subprocesses
	os.Setenv("SELO_OPENCODE_BIN", ocBin)
	os.Setenv("SELO_OPENCODE_MODEL", ocModel)
	os.Setenv("SELO_SERVE_TIMEOUT", "30")
	os.Setenv("SELO_RUN_TIMEOUT_SEC", "180")

	startTime := time.Now()
	summary := &soak.SoakSummary{
		SoakID:    fmt.Sprintf("opencode-soak-%d", time.Now().Unix()),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	var taskDurations []float64
	processed := 0
	errors := 0

	for i, task := range tasks {
		select {
		case <-time.After(cfg.Duration):
			goto done
		default:
		}

		if cfg.TaskCount > 0 && i >= cfg.TaskCount {
			break
		}

		taskStarted := time.Now()
		taskDir := filepath.Join(cfg.OutDir, fmt.Sprintf("task-%d", i))
		os.RemoveAll(taskDir)
		os.MkdirAll(taskDir, 0755)
		for _, d := range []string{"queue/pending", "queue/running", "queue/done", "queue/failed", "queue/review", "receipts", "runs", "worktrees", "config"} {
			os.MkdirAll(filepath.Join(taskDir, d), 0755)
		}

		// Write task file
		taskPath := filepath.Join(taskDir, "queue", "pending", "task.md")
		writeSoakTaskFile(taskPath, task)
		summary.TasksGenerated++

		// Process with daemon
		qm := queue.NewQueueManager(taskDir)
		rw := receipt.NewReceiptWriter(qm.ReceiptsDir(), qm.RunsDir())
		wtm := workspace.NewWorktreeManager(filepath.Join(taskDir, "worktrees"))

		taskCfg := &Config{}
		taskCfg.Forge.DefaultMaxMinutes = task.MaxMinutes
		taskCfg.Forge.DefaultMaxRounds = 1
		taskCfg.Forge.DefaultMaxFiles = 10
		taskCfg.Forge.DefaultMaxPatchLines = 200
		taskCfg.Forge.Runner.Mode = "real"
		taskCfg.Forge.Runner.Command = filepath.Join(os.Getenv("HOME"), "C1-forge", "scripts", "opencode-adapter.sh")
		taskCfg.Forge.Runner.Args = []string{"--task-file", "{{task_file}}", "--workdir", "{{worktree}}", "--max-minutes", "{{max_minutes}}"}
		taskCfg.Forge.ForbiddenClaims = []string{"PROFITABLE", "LIVE_READY"}
		taskCfg.Forge.Notify = "stdout"

		processedResult := processOneTask(qm, rw, wtm, taskCfg)
		if !processedResult {
			errors++
			writeHeartbeat(heartbeatFile, "error", taskDir)
			continue
		}

		// Read receipt
		rec := readNewestReceiptJSON(qm.ReceiptsDir())
		if rec == nil {
			errors++
			summary.MissingReceipts++
			writeHeartbeat(heartbeatFile, "missing_receipt", taskDir)
			continue
		}

		processed++
		duration := time.Since(taskStarted).Seconds()
		taskDurations = append(taskDurations, duration)

		// Extract receipt fields
		verdict := getJSONString(rec, "final_verdict")
		summary.ReceiptsWritten++

		// Classify outcome
		switch verdict {
		case "SUCCESS_WITH_RECEIPT":
			summary.TasksSuccess++
		case "NOOP_WITH_RECEIPT":
			summary.TasksNoop++
		case "PARTIAL_FAILURE":
			summary.TasksPartialFailure++
		case "NEEDS_HUMAN":
			summary.TasksNeedsHuman++
		case "FAILED_SAFETY":
			summary.TasksFailedSafety++
		case "FAILED_TIMEOUT":
			summary.TasksFailedTimeout++
		case "FAILED_INTERNAL_ERROR":
			summary.TasksInternalError++
		}

		// Track overrides
		if getJSONBool(rec, "verdict_overridden") {
			summary.VerdictOverrides++
		}

		// Track test integrity
		if !getJSONBool(rec, "test_integrity_passed") {
			summary.TestIntegrityFailures++
		}
		if len(getJSONStringSlice(rec, "tests_removed")) > 0 {
			summary.TestDeletionsDetected++
		}

		// Track false success
		if verdict == "SUCCESS_WITH_RECEIPT" && (!getJSONBool(rec, "test_integrity_passed") || !getJSONBool(rec, "scans_passed")) {
			summary.FalseSuccessCount++
			summary.FalseSuccessCaught++
		}

		// Track forbidden file hits
		if !getJSONBool(rec, "scans_passed") && verdict != "SUCCESS_WITH_RECEIPT" {
			summary.ForbiddenFileHits++
			summary.SecretScanHits++
		}

		// Useful patch classification
		diff := getJSONString(rec, "diff")
		diffLen := len(diff)
		tiPassed := getJSONBool(rec, "test_integrity_passed")
		scansPassed := getJSONBool(rec, "scans_passed")
		if verdict == "SUCCESS_WITH_RECEIPT" && diffLen > 0 && tiPassed && scansPassed {
			summary.UsefulPatchCount++
		}
		if diffLen > 0 && tiPassed && scansPassed && (verdict == "PARTIAL_FAILURE" || verdict == "NEEDS_HUMAN") {
			summary.PartialUsefulCount++
		}

		// Timing
		if duration > summary.MaxTaskDurationSec {
			summary.MaxTaskDurationSec = duration
		}

		// Check for orphan OpenCode processes
		if checkOrphanOpenCodeProcesses() {
			summary.OrphanOpenCodeProcesses++
		}

		fmt.Printf("[selo] Task %d/%d: %s -> %s (%.1fs)\n", i+1, len(tasks), task.ID, verdict, duration)
		writeHeartbeat(heartbeatFile, "completed", taskDir)
	}

done:
	// Compute summary
	summary.TasksClaimed = processed
	summary.TasksCompleted = processed
	if len(taskDurations) > 0 {
		total := 0.0
		for _, d := range taskDurations {
			total += d
		}
		summary.AvgTaskDurationSec = total / float64(len(taskDurations))
	}
	summary.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	summary.DurationSeconds = int64(time.Since(startTime).Seconds())

	// Useful patch rate
	if processed > 0 {
		summary.UsefulPatchRate = float64(summary.UsefulPatchCount) / float64(processed)
	}

	// Orphan worktrees
	summary.OrphanWorktreesFound = soak.DetectOrphanWorktrees(cfg.OutDir)

	// Determine final verdict
	switch {
	case summary.FalseSuccessCount > 0:
		summary.FinalVerdict = "FAIL_UNSAFE"
	case summary.MissingReceipts > 0:
		summary.FinalVerdict = "FAIL_DAEMON_UNSTABLE"
	case summary.OrphanOpenCodeProcesses > 0:
		summary.FinalVerdict = "FAIL_UNSAFE"
	case summary.UsefulPatchCount >= 2:
		summary.FinalVerdict = "PASS_SELO_PHASE0_9_REAL_OPENCODE_LOCAL_SOAK"
	case summary.UsefulPatchCount > 0:
		summary.FinalVerdict = "PASS_WITH_GAPS_REAL_OPENCODE_LOW_PATCH_RATE"
	default:
		summary.FinalVerdict = "PASS_WITH_GAPS_REAL_OPENCODE_LOW_PATCH_RATE"
	}

	// Write summary
	soak.WriteSummaryJSON(cfg.OutDir, summary)
	soak.WriteSummaryMD(cfg.OutDir, summary)

	fmt.Printf("\n=== OpenCode Soak Complete ===\n")
	fmt.Printf("Tasks: %d processed, %d errors\n", processed, errors)
	fmt.Printf("Useful patches: %d (rate: %.1f%%)\n", summary.UsefulPatchCount, summary.UsefulPatchRate*100)
	fmt.Printf("False successes: %d\n", summary.FalseSuccessCount)
	fmt.Printf("Test integrity failures: %d\n", summary.TestIntegrityFailures)
	fmt.Printf("Orphan OpenCode processes: %d\n", summary.OrphanOpenCodeProcesses)
	fmt.Printf("Final verdict: %s\n", summary.FinalVerdict)
	fmt.Printf("Summary: %s/SOAK_SUMMARY.json\n", cfg.OutDir)

	if summary.FinalVerdict == "FAIL_UNSAFE" || summary.FinalVerdict == "FAIL_DAEMON_UNSTABLE" {
		os.Exit(1)
	}
}

// git helpers for fixture repo creation
func initGitRepo(path string) {
	os.MkdirAll(path, 0755)
	exec.Command("git", "init", path).Run()
	exec.Command("git", "-C", path, "config", "user.email", "test@selo.local").Run()
	exec.Command("git", "-C", path, "config", "user.name", "Selo Test").Run()
}

func writeFile(path, content string) {
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
}

func gitAddCommit(path, msg string) {
	exec.Command("git", "-C", path, "add", ".").Run()
	exec.Command("git", "-C", path, "commit", "-m", msg).Run()
}

// getJSONString extracts a string from a JSON map, returning "" if missing or wrong type.
func getJSONString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getJSONBool extracts a bool from a JSON map.
func getJSONBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// getJSONStringSlice extracts a []string from a JSON map ([]interface{} with string values).
func getJSONStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

// writeHeartbeat writes a heartbeat line to the heartbeat file.
func writeHeartbeat(f *os.File, state string, taskDir string) {
	hb := soak.Heartbeat{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		DaemonState: state,
	}
	data, _ := json.Marshal(hb)
	fmt.Fprintln(f, string(data))
}

// pinocchioVerdictSeverity returns the severity level for verdict comparison.
// Empty string (no recommendation) has severity 0.
func pinocchioVerdictSeverity(v string) int {
	switch v {
	case "":
		return 0
	case "NOOP_WITH_RECEIPT":
		return 0
	case "SUCCESS_WITH_RECEIPT":
		return 1
	case "PARTIAL_FAILURE":
		return 2
	case "NEEDS_HUMAN":
		return 3
	case "FAILED_TIMEOUT":
		return 4
	case "FAILED_LIMIT_EXCEEDED":
		return 5
	case "FAILED_SAFETY":
		return 6
	case "FAILED_INTERNAL_ERROR":
		return 7
	default:
		return 3
	}
}

// formatOverrideReason summarises why Pinocchio overrode the initial verdict.
func formatOverrideReason(p *pinocchio.PinocchioResult) string {
	if len(p.FalseClaims) > 0 {
		return "false claims detected: " + p.FalseClaims[0]
	}
	if len(p.SafetyFindings) > 0 {
		return "safety findings: " + p.SafetyFindings[0]
	}
	if len(p.Inconsistencies) > 0 {
		return "inconsistencies: " + p.Inconsistencies[0]
	}
	return "verdict upgraded by Pinocchio consistency gate"
}

// notificationResult captures the outcome of sending a notification.
type notificationResult struct {
	success bool
	errMsg  string
}

// resolvedNotifyMode returns the effective notification mode from config.
func resolvedNotifyMode(cfg *Config) string {
	mode := cfg.Forge.NotifyConfig.Mode
	if mode == "" {
		mode = cfg.Forge.Notify
	}
	if mode == "" {
		mode = "stdout"
	}
	return mode
}

// sendNotification creates a notifier from config and sends a notification.
func sendNotification(ctx context.Context, cfg *Config, forgeReceipt *receipt.ForgeReceipt, taskMeta *receipt.TaskMeta, reviewPath, runsDir string) notificationResult {
	notifier, warn := notify.NewNotifierFromConfig(cfg.Forge.NotifyConfig)
	if warn != "" {
		return notificationResult{false, warn}
	}

	// Build receipt path
	receiptPath := ""
	if runsDir != "" && forgeReceipt.TaskID != "" {
		receiptPath = filepath.Join(runsDir, fmt.Sprintf("run-%s", forgeReceipt.TaskID), "receipt.json")
	}

	n := notify.Notification{
		TaskID:            forgeReceipt.TaskID,
		FinalVerdict:      forgeReceipt.FinalVerdict,
		InitialVerdict:    forgeReceipt.InitialVerdict,
		VerdictOverridden: forgeReceipt.VerdictOverridden,
		TestsPassed:       forgeReceipt.TestsPassed > 0,
		ScansPassed:       forgeReceipt.ScansPassed,
		FilesChanged:      forgeReceipt.FilesChanged,
		ReceiptPath:       receiptPath,
		ReviewPath:        reviewPath,
		NeedsHuman:        forgeReceipt.FinalVerdict == receipt.VerdictNeedsHuman || forgeReceipt.FinalVerdict == receipt.VerdictPartial,
		SafetyFailure:     forgeReceipt.FinalVerdict == receipt.VerdictSafety,
	}
	if taskMeta != nil {
		n.Repo = taskMeta.Repo
	}

	if err := notifier.Notify(ctx, n); err != nil {
		return notificationResult{false, err.Error()}
	}
	return notificationResult{true, ""}
}

// updateReceiptNotification rewrites the receipt JSON files with notification outcome.
// Re-signs the receipt since notification fields are part of canonical JSON.
func updateReceiptNotification(runsDir, taskID string, forgeReceipt *receipt.ForgeReceipt) {
	if runsDir == "" || taskID == "" {
		return
	}
	// Re-sign after notification fields mutated (canonical excludes signature)
	forgeReceipt.Signature = ""
	forgeReceipt.PublicKey = ""
	forgeReceipt.ReceiptHash = ""
	if _, err := receipt.SignReceipt(forgeReceipt); err != nil {
		fmt.Fprintf(os.Stderr, "[selo] re-sign failed: %v\n", err)
	}
	jsonData, err := json.MarshalIndent(forgeReceipt, "", "  ")
	if err != nil {
		return
	}
	// Update runs dir
	runDir := filepath.Join(runsDir, fmt.Sprintf("run-%s", taskID))
	os.WriteFile(filepath.Join(runDir, "receipt.json"), jsonData, 0644)
	// Update receipts dir if it's a sibling of runs
	receiptsDir := filepath.Join(filepath.Dir(runsDir), "receipts")
	if entries, err := os.ReadDir(receiptsDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), forgeReceipt.ReceiptID) && strings.HasSuffix(e.Name(), ".json") {
				os.WriteFile(filepath.Join(receiptsDir, e.Name()), jsonData, 0644)
				return
			}
		}
	}
}

func findBaseDir(configPath string) (string, error) {
	if strings.HasPrefix(configPath, "/") {
		// Absolute path: base dir is the dir containing config /selo.yaml
		base := filepath.Dir(configPath)
		if strings.HasSuffix(base, "/config") || strings.HasSuffix(base, "\\config") {
			return filepath.Dir(base), nil
		}
		return base, nil
	}
	// Relative path: try common locations
	candidates := []string{
		".",
		filepath.Join(os.Getenv("HOME"), "C1-forge"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, configPath)); err == nil {
			return c, nil
		}
	}
	return ".", nil
}

func loadConfig(configPath string, baseDir string) *Config {
	cfg := &Config{}
	cfg.Forge.PollIntervalSec = 5
	cfg.Forge.DefaultMaxMinutes = 30
	cfg.Forge.DefaultMaxRounds = 3
	cfg.Forge.DefaultMaxFiles = 10
	cfg.Forge.DefaultMaxPatchLines = 200
	cfg.Forge.StopFile = ".selo-stop"
	cfg.Forge.LockFile = ".selo.lock"
	cfg.Forge.Notify = "stdout"
	cfg.Forge.NotifyConfig.Mode = "stdout"
	cfg.Forge.NotifyConfig.TimeoutSeconds = 5
	cfg.Forge.ForbiddenClaims = []string{
		"PROFITABLE", "LIVE_READY", "MONEY_ENGINE",
		"CAPITAL_APPROVED", "LIVE_CAPITAL_APPROVED", "MONEY_MACHINE",
	}

	// Try to read YAML config (simple implementation without yaml dependency)
	stripQ := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "\"")
		s = strings.Trim(s, "'")
		return s
	}

	fullPath := filepath.Join(baseDir, configPath)
	if data, err := os.ReadFile(fullPath); err == nil {
		content := string(data)
		lines := strings.Split(content, "\n")

		// Extract runner/containment sections
		inRunner := false
		inContainment := false
		inArgs := false
		inNotifyConfig := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Detect notify_config section boundaries
			if trimmed == "notify_config:" {
				inNotifyConfig = true
				continue
			}
			if inNotifyConfig && trimmed != "" && !strings.HasPrefix(trimmed, "notify_config") &&
				!strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(trimmed, ":") {
				inNotifyConfig = false
			}

			if strings.TrimSpace(line) == "containment:" {
				inContainment = true
				continue
			}
			if inContainment {
				if strings.HasPrefix(trimmed, "strategy:") {
					cfg.Forge.Containment.Strategy = stripQ(strings.TrimPrefix(trimmed, "strategy:"))
				}
				if strings.HasPrefix(trimmed, "image:") {
					cfg.Forge.Containment.Image = stripQ(strings.TrimPrefix(trimmed, "image:"))
				}
				if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") && trimmed != "containment:" {
					inContainment = false
				} else if strings.TrimSpace(line) == "runner:" || strings.TrimSpace(line) == "notify_config:" || strings.HasPrefix(trimmed, "poll_interval") {
					inContainment = false
				}
				if inContainment {
					continue
				}
			}

			if strings.TrimSpace(line) == "runner:" {
				inRunner = true
				inArgs = false
				continue
			}

			if inRunner {
				// Detect args section
				if strings.Contains(trimmed, "args:") {
					inArgs = true
					continue
				}
				// Detect end of args list
				if inArgs && trimmed != "" && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "#") {
					inArgs = false
				}
				// Collect args
				if inArgs && strings.HasPrefix(trimmed, "- ") {
					arg := stripQ(strings.TrimPrefix(trimmed, "- "))
					cfg.Forge.Runner.Args = append(cfg.Forge.Runner.Args, arg)
					continue
				}
				// Parse runner key-value pairs
				if strings.HasPrefix(trimmed, "mode:") {
					cfg.Forge.Runner.Mode = stripQ(strings.TrimPrefix(trimmed, "mode:"))
				}
				if strings.HasPrefix(trimmed, "command:") {
					cfg.Forge.Runner.Command = stripQ(strings.TrimPrefix(trimmed, "command:"))
				}
				// Exit runner when we hit a top-level key (not indented)
				if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") && trimmed != "runner:" {
					inRunner = false
				}
				continue
			}

			// Top-level keys
			if strings.HasPrefix(trimmed, "poll_interval_sec:") {
				fmt.Sscanf(trimmed, "poll_interval_sec: %d", &cfg.Forge.PollIntervalSec)
			}
			if strings.HasPrefix(trimmed, "default_max_minutes:") {
				fmt.Sscanf(trimmed, "default_max_minutes: %d", &cfg.Forge.DefaultMaxMinutes)
			}
			if strings.HasPrefix(trimmed, "default_max_rounds:") {
				fmt.Sscanf(trimmed, "default_max_rounds: %d", &cfg.Forge.DefaultMaxRounds)
			}
			if strings.HasPrefix(trimmed, "notify:") {
				cfg.Forge.Notify = stripQ(strings.TrimPrefix(trimmed, "notify:"))
			}
			if strings.HasPrefix(trimmed, "notify_config:") {
				// Skip the section header; sub-keys handled below
			}
			if strings.HasPrefix(trimmed, "mode:") && inNotifyConfig {
				cfg.Forge.NotifyConfig.Mode = stripQ(strings.TrimPrefix(trimmed, "mode:"))
			}
			if strings.HasPrefix(trimmed, "ntfy_url:") && inNotifyConfig {
				cfg.Forge.NotifyConfig.NtfyURL = stripQ(strings.TrimPrefix(trimmed, "ntfy_url:"))
			}
			if strings.HasPrefix(trimmed, "timeout_seconds:") && inNotifyConfig {
				fmt.Sscanf(trimmed, "timeout_seconds: %d", &cfg.Forge.NotifyConfig.TimeoutSeconds)
			}
		}
	}

	// Migrate legacy notify field to NotifyConfig if NotifyConfig.Mode is still default
	if cfg.Forge.NotifyConfig.Mode == "stdout" && cfg.Forge.Notify != "stdout" {
		cfg.Forge.NotifyConfig.Mode = cfg.Forge.Notify
	}
	// If legacy ntfy_topic is set and no explicit ntfy_url, derive one
	if cfg.Forge.NtfyTopic != "" && cfg.Forge.NotifyConfig.NtfyURL == "" {
		cfg.Forge.NotifyConfig.NtfyURL = "https://ntfy.sh/" + cfg.Forge.NtfyTopic
	}

	if cfg.Forge.Containment.Strategy == "" {
		cfg.Forge.Containment.Strategy = "worktree"
	}
	if cfg.Forge.Containment.Image == "" && cfg.Forge.Containment.Strategy == "docker" {
		cfg.Forge.Containment.Image = "alpine:latest"
	}

	// Security: environment variable override for dangerously_skip_permissions
	// This setting is restricted to:
	// 1. Environment variable override only (not config file)
	// 2. Debug builds only (when build debug flag is set)
	// 3. Must be explicitly set to "1" or "true"
	if os.Getenv("SELO_DANGEROUSLY_SKIP_PERMISSIONS") == "1" ||
		os.Getenv("SELO_DANGEROUSLY_SKIP_PERMISSIONS") == "true" {
		// Check if this is a debug build
		if isDebugBuild() {
			cfg.Forge.OpenCode.DangerouslySkipPermissions = true
		} else {
			fmt.Fprintln(os.Stderr, "[selo] ⚠️  WARNING: dangerously_skip_permissions is only allowed in debug builds")
			fmt.Fprintln(os.Stderr, "[selo]    Build with -tags=debug to enable this feature")
		}
	}

	// Security: warn when dangerously_skip_permissions is active
	if cfg.Forge.OpenCode.DangerouslySkipPermissions {
		fmt.Fprintln(os.Stderr, "[selo] ⚠️  SECURITY WARNING: dangerously_skip_permissions is enabled")
		fmt.Fprintln(os.Stderr, "[selo]    The agent will have unrestricted file system access.")
		fmt.Fprintln(os.Stderr, "[selo]    Use only in isolated environments (containers, worktrees).")
		fmt.Fprintln(os.Stderr, "[selo]    Set SELO_DANGEROUSLY_SKIP_PERMISSIONS=0 to disable.")
	}

	return cfg
}

// isDebugBuild checks if this is a debug build.
// Debug builds are created with: go build -tags=debug
func isDebugBuild() bool {
	// This will be set by ldflags during build
	return debugBuild == "true"
}

var debugBuild = "false"

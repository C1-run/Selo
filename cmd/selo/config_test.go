package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/desmondkam/selo/internal/receipt"
	"github.com/desmondkam/selo/internal/soak"
)

// writeConfig writes a config file under <dir>/config/selo.yaml and returns dir.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "selo.yaml"), []byte(body), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir
}

// Every key a user can set in config/selo.yaml must actually reach the Config
// struct. The previous hand-rolled line scanner silently dropped whole sections —
// notably forge.opencode.*, default_max_files and default_max_patch_lines — so a
// config that looked applied had no effect.
func TestLoadConfigPopulatesEveryDocumentedSection(t *testing.T) {
	body := `forge:
  poll_interval_sec: 7
  default_max_minutes: 11
  default_max_rounds: 2
  default_max_files: 33
  default_max_patch_lines: 44
  stop_file: ".my-stop"
  lock_file: ".my.lock"
  notify_config:
    mode: "ntfy"
    ntfy_url: "https://ntfy.sh/example"
    timeout_seconds: 9
  forbidden_claims:
    - "NEVER_CLAIM_THIS"
  runner:
    mode: "opencode"
    command: "opencode"
    args:
      - "run"
      - "{{task_file}}"
  opencode:
    model: "anthropic/claude-sonnet-4-5"
    agent: "build"
    serve_timeout: 45
    dangerously_skip_permissions: false
    permission_allowlist:
      - "/project/src/*.go"
  containment:
    strategy: "worktree"
`
	dir := writeConfig(t, body)
	cfg := loadConfig("config/selo.yaml", dir)

	if got := cfg.Forge.PollIntervalSec; got != 7 {
		t.Errorf("PollIntervalSec = %d, want 7", got)
	}
	if got := cfg.Forge.DefaultMaxMinutes; got != 11 {
		t.Errorf("DefaultMaxMinutes = %d, want 11", got)
	}
	if got := cfg.Forge.DefaultMaxFiles; got != 33 {
		t.Errorf("DefaultMaxFiles = %d, want 33 (was never parsed before)", got)
	}
	if got := cfg.Forge.DefaultMaxPatchLines; got != 44 {
		t.Errorf("DefaultMaxPatchLines = %d, want 44 (was never parsed before)", got)
	}
	if got := cfg.Forge.StopFile; got != ".my-stop" {
		t.Errorf("StopFile = %q, want .my-stop", got)
	}
	if got := cfg.Forge.Runner.Mode; got != "opencode" {
		t.Errorf("Runner.Mode = %q, want opencode", got)
	}
	if got := cfg.Forge.Runner.Args; len(got) != 2 || got[0] != "run" {
		t.Errorf("Runner.Args = %v, want [run {{task_file}}]", got)
	}
	if got := cfg.Forge.OpenCode.Model; got != "anthropic/claude-sonnet-4-5" {
		t.Errorf("OpenCode.Model = %q, want anthropic/claude-sonnet-4-5 (was never parsed before)", got)
	}
	if got := cfg.Forge.OpenCode.Agent; got != "build" {
		t.Errorf("OpenCode.Agent = %q, want build", got)
	}
	if got := cfg.Forge.OpenCode.ServeTimeout; got != 45 {
		t.Errorf("OpenCode.ServeTimeout = %d, want 45", got)
	}
	if got := cfg.Forge.OpenCode.PermissionAllowlist; len(got) != 1 || got[0] != "/project/src/*.go" {
		t.Errorf("OpenCode.PermissionAllowlist = %v, want [/project/src/*.go]", got)
	}
	if got := cfg.Forge.NotifyConfig.Mode; got != "ntfy" {
		t.Errorf("NotifyConfig.Mode = %q, want ntfy", got)
	}
	if got := cfg.Forge.NotifyConfig.NtfyURL; got != "https://ntfy.sh/example" {
		t.Errorf("NotifyConfig.NtfyURL = %q, want https://ntfy.sh/example", got)
	}
	if got := cfg.Forge.NotifyConfig.TimeoutSeconds; got != 9 {
		t.Errorf("NotifyConfig.TimeoutSeconds = %d, want 9", got)
	}
	if got := cfg.Forge.ForbiddenClaims; len(got) != 1 || got[0] != "NEVER_CLAIM_THIS" {
		t.Errorf("ForbiddenClaims = %v, want [NEVER_CLAIM_THIS]", got)
	}
}

// Absent keys must keep their defaults rather than becoming zero values.
func TestLoadConfigKeepsDefaultsForAbsentKeys(t *testing.T) {
	dir := writeConfig(t, "forge:\n  poll_interval_sec: 7\n")
	cfg := loadConfig("config/selo.yaml", dir)

	if got := cfg.Forge.DefaultMaxMinutes; got != 30 {
		t.Errorf("DefaultMaxMinutes = %d, want default 30", got)
	}
	if got := cfg.Forge.StopFile; got != ".selo-stop" {
		t.Errorf("StopFile = %q, want default .selo-stop", got)
	}
	if got := cfg.Forge.Containment.Strategy; got != "worktree" {
		t.Errorf("Containment.Strategy = %q, want default worktree", got)
	}
	if cfg.Forge.Runner.Mode != "" {
		t.Errorf("Runner.Mode = %q, want empty (no default)", cfg.Forge.Runner.Mode)
	}
}

// A missing config file is normal (defaults only) and must not panic.
func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	cfg := loadConfig("config/selo.yaml", t.TempDir())
	if cfg.Forge.PollIntervalSec != 5 || cfg.Forge.DefaultMaxMinutes != 30 {
		t.Errorf("defaults not applied: poll=%d max=%d", cfg.Forge.PollIntervalSec, cfg.Forge.DefaultMaxMinutes)
	}
}

func TestUnknownConfigKeysAreDetected(t *testing.T) {
	body := `forge:
  poll_interval_sec: 5
  bogus_setting: 42
  runner:
    mode: "mock"
    not_a_real_key: "x"
scan_tools:
  secret_scan: "gone"
`
	dir := writeConfig(t, body)
	data, err := os.ReadFile(filepath.Join(dir, "config", "selo.yaml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cfg := &Config{}
	got := unknownConfigKeys(data, cfg)

	want := map[string]bool{
		"forge.bogus_setting":         true,
		"forge.runner.not_a_real_key": true,
		"scan_tools":                  true,
	}
	if len(got) != len(want) {
		t.Fatalf("unknownConfigKeys = %v, want %d entries", got, len(want))
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected unknown key %q", k)
		}
	}
}

func TestUnknownConfigKeysAcceptsKnownConfig(t *testing.T) {
	data, err := os.ReadFile("../../config/selo.example.yaml")
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	if got := unknownConfigKeys(data, &Config{}); len(got) > 0 {
		t.Errorf("config/selo.example.yaml advertises keys Selo does not read: %v", got)
	}
}

// Refusing is the point: running with weaker isolation than configured produces
// a receipt that looks fine and is not.
func TestValidateConfigRejectsUnimplementedIsolation(t *testing.T) {
	base := &Config{}
	base.Forge.Runner.Mode = "mock"

	for _, strategy := range []string{"docker", "local"} {
		cfg := *base
		cfg.Forge.Containment.Strategy = strategy
		if err := validateConfig(&cfg); err == nil {
			t.Errorf("containment.strategy %q was accepted, want refusal", strategy)
		}
	}

	ok := *base
	ok.Forge.Containment.Strategy = "worktree"
	if err := validateConfig(&ok); err != nil {
		t.Errorf("worktree strategy rejected: %v", err)
	}

	empty := *base
	empty.Forge.Containment.Strategy = ""
	if err := validateConfig(&empty); err != nil {
		t.Errorf("empty strategy should default to worktree, got: %v", err)
	}
}

func TestValidateConfigRejectsUnknownRunnerMode(t *testing.T) {
	cfg := &Config{}
	cfg.Forge.Containment.Strategy = "worktree"

	for _, mode := range []string{"mock", "opencode", ""} {
		cfg.Forge.Runner.Mode = mode
		if err := validateConfig(cfg); err != nil {
			t.Errorf("runner.mode %q rejected: %v", mode, err)
		}
	}

	cfg.Forge.Runner.Mode = "custom"
	if err := validateConfig(cfg); err == nil {
		t.Error("runner.mode \"custom\" accepted — it silently degrades to mock")
	}
}

// selo init must create the directories the runtime writes to. It used to create
// a parallel .selo/ tree the queue manager never touches.
func TestInitCreatesRuntimeDirsAndGitignore(t *testing.T) {
	dir := t.TempDir()

	prevRepo := initRepo
	initRepo = dir
	defer func() { initRepo = prevRepo }()

	if err := runInitCmd(nil, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	for _, d := range []string{
		"queue/pending", "queue/running", "queue/review", "queue/done", "queue/failed",
		"runs", "receipts", "worktrees", "config",
	} {
		if st, err := os.Stat(filepath.Join(dir, d)); err != nil || !st.IsDir() {
			t.Errorf("init did not create %s/", d)
		}
	}

	// The queue directory is written on every run; without it the user's repo
	// picks up untracked state.
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, entry := range []string{"queue/", "runs/", "receipts/", "worktrees/"} {
		if !strings.Contains(string(data), entry) {
			t.Errorf(".gitignore missing %q", entry)
		}
	}

	// The config init writes must survive its own validator, or the documented
	// quickstart fails on the first command.
	cfg := loadConfig("config/selo.yaml", dir)
	if err := validateConfig(cfg); err != nil {
		t.Errorf("config written by selo init is rejected by validateConfig: %v", err)
	}
}

// The soak fixtures are fed through the real pipeline, so they must parse as real
// tasks. They did not: a bare "[]" list is rejected by the task parser.
func TestSoakFixtureTasksParseAsRealTasks(t *testing.T) {
	dir := t.TempDir()
	for _, task := range soak.GenerateFixtureTasks(6) {
		if err := soak.WriteFixtureTask(dir, task); err != nil {
			t.Fatalf("WriteFixtureTask(%s): %v", task.ID, err)
		}
		path := filepath.Join(dir, task.ID+".md")
		meta, err := receipt.ParseTaskMeta(path)
		if err != nil {
			t.Errorf("fixture task %s does not parse: %v", task.ID, err)
			continue
		}
		if meta.Goal != task.Goal {
			t.Errorf("fixture %s: goal = %q, want %q", task.ID, meta.Goal, task.Goal)
		}
		if len(meta.Commands) != len(task.Commands) {
			t.Errorf("fixture %s: commands = %v, want %v", task.ID, meta.Commands, task.Commands)
		}
	}
}

// The goal mix must be reproducible so two soak summaries can be compared.
func TestGenerateFixtureTasksIsDeterministic(t *testing.T) {
	first := soak.GenerateFixtureTasks(20)
	second := soak.GenerateFixtureTasks(20)
	for i := range first {
		if first[i].Goal != second[i].Goal || first[i].ID != second[i].ID {
			t.Fatalf("task %d differs between runs: %q vs %q", i, first[i].Goal, second[i].Goal)
		}
	}
}

// The default forbidden-claim list exists in two places — the Go default and the
// YAML template `selo init` writes — so a test has to hold them together or they
// will drift.
func TestInitWritesTheSameDefaultClaimsAsLoadConfig(t *testing.T) {
	dir := t.TempDir()

	prevRepo := initRepo
	initRepo = dir
	defer func() { initRepo = prevRepo }()

	if err := runInitCmd(nil, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	fromInit := loadConfig("config/selo.yaml", dir).Forge.ForbiddenClaims
	if len(fromInit) == 0 {
		t.Fatal("config written by selo init has no forbidden_claims")
	}

	// loadConfig's own defaults, with no config file present.
	bare := loadConfig("config/selo.yaml", t.TempDir()).Forge.ForbiddenClaims

	if len(fromInit) != len(bare) {
		t.Fatalf("init wrote %d default claims, loadConfig defaults to %d: %v vs %v",
			len(fromInit), len(bare), fromInit, bare)
	}
	for i, want := range bare {
		if fromInit[i] != want {
			t.Errorf("claim %d: init wrote %q, default is %q", i, fromInit[i], want)
		}
	}

	// The shipped defaults must be generic overclaims, not the retired
	// domain-specific tokens.
	for _, retired := range []string{"PROFITABLE", "MONEY_ENGINE", "MONEY_MACHINE"} {
		for _, got := range fromInit {
			if got == retired {
				t.Errorf("retired domain-specific claim %q is still a shipped default", retired)
			}
		}
	}
}

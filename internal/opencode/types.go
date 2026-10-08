package opencode

// BinaryInfo holds information about the discovered OpenCode binary.
type BinaryInfo struct {
	Path     string
	Version  string
	Verified bool
}

// ServerOpts configures the OpenCode serve process.
type ServerOpts struct {
	BinaryPath  string
	Port        int // 0 = auto-assign
	WorkDir     string
	Model       string
	Agent       string
	Permissions bool     // dangerously-skip-permissions
	Allowlist   []string // permitted file patterns when permissions=false
}

// RunResult captures the outcome of a one-shot OpenCode run.
type RunResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	TimedOut   bool
	SessionID  string
	DurationMs int64
}

// Session represents an OpenCode session.
type Session struct {
	ID string `json:"id"`
}

// PromptRequest is the request body for prompting a session.
type PromptRequest struct {
	Prompt string `json:"prompt"`
}

// PromptResponse is the response from prompting a session.
type PromptResponse struct {
	Messages []Message `json:"messages"`
}

// Message represents a message in an OpenCode session.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RunInfo is the JSON structure written to opencode-run-info.json.
type RunInfo struct {
	BinaryPath       string `json:"binary_path"`
	ExitCode         int    `json:"exit_code"`
	Model            string `json:"model"`
	Agent            string `json:"agent"`
	ServerPID        *int   `json:"server_pid,omitempty"`
	ServerStarted    string `json:"server_started,omitempty"`
	ServerKilled     string `json:"server_killed,omitempty"`
	ServerExitStatus *int   `json:"server_exit_status,omitempty"`
	RunExitStatus    *int   `json:"run_exit_status,omitempty"`
	TimeoutHit       bool   `json:"timeout_hit"`
}

// AdapterOpts configures the C1 Forge adapter for OpenCode.
type AdapterOpts struct {
	BinaryPath      string
	WorkDir         string
	TaskFilePath    string
	Model           string
	Agent           string
	MaxMinutes      int
	ServeTimeoutSec int
	RunTimeoutSec   int
	Permissions     bool
	Allowlist       []string // permitted file patterns
}

package notify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Notification carries all task result data for the notifier.
type Notification struct {
	TaskID            string
	Repo              string
	InitialVerdict    string
	FinalVerdict      string
	VerdictOverridden bool
	TestsPassed       bool
	ScansPassed       bool
	FilesChanged      int
	ReceiptPath       string
	ReviewPath        string
	NeedsHuman        bool
	SafetyFailure     bool
	Message           string
}

// Notifier sends task notifications.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// StdoutNotifier writes notifications to stdout.
type StdoutNotifier struct {
	Writer io.Writer
}

func NewStdoutNotifier() *StdoutNotifier {
	return &StdoutNotifier{Writer: os.Stdout}
}

func (s *StdoutNotifier) Notify(_ context.Context, n Notification) error {
	prefix := severityPrefix(n.FinalVerdict)
	msg := fmt.Sprintf("%s: task %s → %s", prefix, n.TaskID, n.FinalVerdict)
	if n.VerdictOverridden {
		msg += " (overridden)"
	}
	if n.ReceiptPath != "" {
		msg += fmt.Sprintf(" | receipt: %s", n.ReceiptPath)
	}
	if n.ReviewPath != "" {
		msg += fmt.Sprintf(" | review: %s", n.ReviewPath)
	}
	_, err := fmt.Fprintln(s.Writer, msg)
	return err
}

// NtfyNotifier sends notifications via HTTP POST to an ntfy topic URL.
type NtfyNotifier struct {
	URL     string
	Timeout time.Duration
	Client  *http.Client
}

func NewNtfyNotifier(url string, timeout time.Duration) *NtfyNotifier {
	return &NtfyNotifier{
		URL:     url,
		Timeout: timeout,
		Client:  &http.Client{Timeout: timeout},
	}
}

func (n *NtfyNotifier) Notify(ctx context.Context, notif Notification) error {
	title := fmt.Sprintf("C1 Forge %s", notif.FinalVerdict)
	var priority int
	switch {
	case notif.SafetyFailure:
		priority = 5
	case notif.NeedsHuman:
		priority = 4
	default:
		priority = 3
	}

	body := newBody(notif)
	req, err := http.NewRequestWithContext(ctx, "POST", n.URL, bytes.NewReader([]byte(body)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", fmt.Sprintf("%d", priority))
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := n.Client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ntfy returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// DisabledNotifier silently discards notifications.
type DisabledNotifier struct{}

func NewDisabledNotifier() *DisabledNotifier {
	return &DisabledNotifier{}
}

func (d *DisabledNotifier) Notify(_ context.Context, n Notification) error {
	return nil
}

func newBody(n Notification) string {
	var b strings.Builder
	b.WriteString("C1 Forge task complete\n")
	b.WriteString(fmt.Sprintf("task: %s\n", n.TaskID))
	if n.Repo != "" {
		b.WriteString(fmt.Sprintf("repo: %s\n", n.Repo))
	}
	b.WriteString(fmt.Sprintf("verdict: %s\n", n.FinalVerdict))
	b.WriteString(fmt.Sprintf("overridden: %v\n", n.VerdictOverridden))
	b.WriteString(fmt.Sprintf("tests: %s\n", passFail(n.TestsPassed)))
	b.WriteString(fmt.Sprintf("scans: %s\n", passFail(n.ScansPassed)))
	b.WriteString(fmt.Sprintf("files: %d\n", n.FilesChanged))
	if n.ReceiptPath != "" {
		b.WriteString(fmt.Sprintf("receipt: %s\n", n.ReceiptPath))
	}
	if n.ReviewPath != "" {
		b.WriteString(fmt.Sprintf("review: %s\n", n.ReviewPath))
	}
	return b.String()
}

func severityPrefix(verdict string) string {
	switch verdict {
	case "SUCCESS_WITH_RECEIPT", "NOOP_WITH_RECEIPT":
		return "✓ Selo"
	case "PARTIAL_FAILURE", "NEEDS_HUMAN":
		return "! Selo"
	case "FAILED_SAFETY":
		return "!! Selo SAFETY"
	case "FAILED_TIMEOUT", "FAILED_LIMIT_EXCEEDED", "FAILED_INTERNAL_ERROR", "FAILED_STALE_LOCK":
		return "X Selo"
	default:
		return "? Selo"
	}
}

func passFail(v bool) string {
	if v {
		return "pass"
	}
	return "fail"
}

// Config holds notification configuration.
type Config struct {
	Mode           string `yaml:"mode"`
	NtfyURL        string `yaml:"ntfy_url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// NewNotifierFromConfig creates a Notifier based on config.
func NewNotifierFromConfig(cfg Config) (Notifier, string) {
	switch cfg.Mode {
	case "ntfy":
		url := cfg.NtfyURL
		if url == "" {
			return NewDisabledNotifier(), "ntfy mode selected but no URL configured; notifications disabled"
		}
		timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		return NewNtfyNotifier(url, timeout), ""
	case "disabled":
		return NewDisabledNotifier(), ""
	default:
		return NewStdoutNotifier(), ""
	}
}

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/C1-run/selo/internal/receipt"
)

// captureStdout runs fn with os.Stdout redirected into a pipe and returns what
// was printed. fmt.Print* resolves os.Stdout at call time, so redirection works.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return string(out)
}

func withReceiptGlobals(t *testing.T, dir, format string, anchor, jsonOut bool) {
	t.Helper()
	oldDir, oldFormat, oldAnchor, oldJSON := receiptDir, receiptFormat, receiptWantAnchor, receiptJSONOut
	receiptDir, receiptFormat, receiptWantAnchor, receiptJSONOut = dir, format, anchor, jsonOut
	t.Cleanup(func() {
		receiptDir, receiptFormat, receiptWantAnchor, receiptJSONOut = oldDir, oldFormat, oldAnchor, oldJSON
	})
}

// archiveSignedReceipt signs a receipt (with a fresh test key) and writes it
// into a temp archive at <base>/receipts/<id>.json.
func archiveSignedReceipt(t *testing.T, mutate func(*receipt.ForgeReceipt)) (base string, r receipt.ForgeReceipt) {
	t.Helper()
	r = signedVerifyTestReceipt(t)
	r.TaskID = "run-testtask"
	r.RunnerMode = "real"
	r.FilesChanged = 3
	r.PatchLines = 120
	r.TestsPassed = 1
	if mutate != nil {
		mutate(&r)
	}
	base = t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "receipts"), 0755); err != nil {
		t.Fatalf("mkdir receipts: %v", err)
	}
	data, err := json.MarshalIndent(&r, "", "  ")
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	path := filepath.Join(base, "receipts", r.ReceiptID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	return base, r
}

// TestRootRegistersAllCommands walks the real cobra command tree. Unit tests
// call RunE functions directly, which bypasses registration — a lost
// rootCmd.AddCommand line compiles fine and only fails at the CLI.
func TestRootRegistersAllCommands(t *testing.T) {
	want := []string{"daemon", "run", "check", "verify", "receipt", "keys", "status", "init", "smoke", "soak", "completion"}
	have := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		have[c.Name()] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("command %q is not registered on the root command", name)
		}
	}
}

func TestReceiptListOutput(t *testing.T) {
	base, r1 := archiveSignedReceipt(t, nil)
	r2 := r1
	r2.ReceiptID = "r-2"
	data, err := json.MarshalIndent(&r2, "", "  ")
	if err != nil {
		t.Fatalf("marshal r2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "receipts", "r-2.json"), data, 0644); err != nil {
		t.Fatalf("write r2: %v", err)
	}
	withReceiptGlobals(t, base, "text", false, false)

	out := captureStdout(t, func() { runReceiptList(nil, nil) })
	if !strings.Contains(out, "r-1") || !strings.Contains(out, "r-2") {
		t.Errorf("list output missing receipt ids: %s", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("list output missing verdict: %s", out)
	}

	withReceiptGlobals(t, base, "text", false, true)
	out = captureStdout(t, func() { runReceiptList(nil, nil) })
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("list --json is not valid JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Errorf("list --json returned %d rows, want 2", len(rows))
	}
}

func TestReceiptShowMarkdown(t *testing.T) {
	base, r := archiveSignedReceipt(t, func(rr *receipt.ForgeReceipt) {
		rr.DiffSummary = "diff --git a/x b/x"
	})
	withReceiptGlobals(t, base, "markdown", false, false)

	out := captureStdout(t, func() { runReceiptShow(nil, []string{r.ReceiptID}) })
	for _, want := range []string{
		"## Selo receipt `" + r.ReceiptID + "`",
		"**Verdict: `PASS`**",
		"Safety findings: none",
		"Files changed: 3",
		"Verify independently: `selo verify",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown card missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<details>") {
		t.Error("plain markdown must not contain <details> (that is the github format)")
	}
}

func TestReceiptShowGithubWrapsDetails(t *testing.T) {
	base, r := archiveSignedReceipt(t, nil)
	withReceiptGlobals(t, base, "github", false, false)

	out := captureStdout(t, func() { runReceiptShow(nil, []string{r.ReceiptID}) })
	if !strings.Contains(out, "<details>") {
		t.Errorf("github format should wrap verbose sections in <details>:\n%s", out)
	}
}

func TestReceiptShowFlagsTampering(t *testing.T) {
	base, r := archiveSignedReceipt(t, nil)
	r.Verdict = "FAIL" // tamper after signing
	data, err := json.MarshalIndent(&r, "", "  ")
	if err != nil {
		t.Fatalf("marshal tampered receipt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "receipts", r.ReceiptID+".json"), data, 0644); err != nil {
		t.Fatalf("rewrite receipt: %v", err)
	}
	withReceiptGlobals(t, base, "text", false, false)

	out := captureStdout(t, func() { runReceiptShow(nil, []string{r.ReceiptID}) })
	for _, want := range []string{"Result:     INVALID", "content hash mismatch", "signature"} {
		if !strings.Contains(out, want) {
			t.Errorf("tampered receipt card missing %q:\n%s", want, out)
		}
	}
}

func TestReceiptShowUnreadable(t *testing.T) {
	base, _ := archiveSignedReceipt(t, nil)
	withReceiptGlobals(t, base, "text", false, false)

	out := captureStdout(t, func() { runReceiptShow(nil, []string{"no-such-receipt"}) })
	if !strings.Contains(out, "INVALID") || !strings.Contains(out, "unreadable receipt") {
		t.Errorf("missing receipt should render an INVALID card with reason:\n%s", out)
	}
}

func TestResolveReceiptPath(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "receipts"), 0755)
	os.MkdirAll(filepath.Join(base, "runs", "run-abc"), 0755)
	os.WriteFile(filepath.Join(base, "receipts", "r-1.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(base, "runs", "run-abc", "receipt.json"), []byte("{}"), 0644)

	if got := resolveReceiptPath("r-1", base); got != filepath.Join(base, "receipts", "r-1.json") {
		t.Errorf("id resolution: %s", got)
	}
	if got := resolveReceiptPath("run-abc", base); got != filepath.Join(base, "runs", "run-abc", "receipt.json") {
		t.Errorf("run-id resolution: %s", got)
	}
	if got := resolveReceiptPath("/abs/path/rec.json", base); got != "/abs/path/rec.json" {
		t.Errorf("path passthrough: %s", got)
	}
}

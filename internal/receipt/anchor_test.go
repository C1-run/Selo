package receipt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAnchorReceipt(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")
	os.MkdirAll(repoDir, 0755)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = repoDir
	cmd.Run()

	// Create a test receipt file
	receiptPath := filepath.Join(tempDir, "test-receipt.json")
	receiptContent := `{"receipt_id": "selo-123456", "verdict": "SUCCESS_WITH_RECEIPT"}`
	if err := os.WriteFile(receiptPath, []byte(receiptContent), 0644); err != nil {
		t.Fatalf("write receipt failed: %v", err)
	}

	// Anchor the receipt
	result, err := AnchorReceipt(receiptPath, repoDir)
	if err != nil {
		t.Fatalf("anchor receipt failed: %v", err)
	}

	// Verify result
	if result.ReceiptHash == "" {
		t.Error("receipt hash is empty")
	}
	if result.AnchorHash == "" {
		t.Error("anchor hash is empty")
	}
	if result.AnchorType != "git_commit" {
		t.Errorf("anchor type = %q, want %q", result.AnchorType, "git_commit")
	}
	if result.GitCommit == "" {
		t.Error("git commit is empty")
	}

	// Verify the anchor was created
	anchorFile := filepath.Join(repoDir, ".selo", "anchors", "anchor-"+result.ReceiptHash[:12]+".txt")
	if _, err := os.Stat(anchorFile); os.IsNotExist(err) {
		t.Error("anchor file was not created")
	}

	// Verify the anchor
	valid, err := VerifyAnchor(repoDir, result.ReceiptHash)
	if err != nil {
		t.Fatalf("verify anchor failed: %v", err)
	}
	if !valid {
		t.Error("anchor verification failed")
	}
}

func TestVerifyAnchor_InvalidHash(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")
	os.MkdirAll(repoDir, 0755)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	cmd.Run()

	// Try to verify with non-existent hash
	valid, err := VerifyAnchor(repoDir, "nonexistent1234567890")
	if err == nil {
		t.Error("expected error for non-existent anchor")
	}
	if valid {
		t.Error("expected invalid result")
	}
}

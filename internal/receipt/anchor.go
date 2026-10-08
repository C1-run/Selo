package receipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AnchorResult represents the result of anchoring a receipt.
type AnchorResult struct {
	ReceiptHash string    `json:"receipt_hash"`
	AnchorHash  string    `json:"anchor_hash"`
	AnchorType  string    `json:"anchor_type"` // "git_commit" | "external"
	AnchoredAt  time.Time `json:"anchored_at"`
	GitCommit   string    `json:"git_commit,omitempty"`
	GitBranch   string    `json:"git_branch,omitempty"`
}

// ensureReceiptAnchorBranch checks out the receipt-anchor branch, creating an orphan if needed.
// Returns the previous branch to restore.
func ensureReceiptAnchorBranch(repoPath string) (string, error) {
	// Get current branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	out, _ := cmd.Output()
	prevBranch := strings.TrimSpace(string(out))
	if prevBranch == "" {
		prevBranch = "main"
	}
	// Check if receipt-anchor exists
	cmd = exec.Command("git", "branch", "--list", "receipt-anchor")
	cmd.Dir = repoPath
	out, _ = cmd.Output()
	if strings.TrimSpace(string(out)) == "" {
		// Create orphan branch
		cmd = exec.Command("git", "checkout", "--orphan", "receipt-anchor")
		cmd.Dir = repoPath
		if _, err := cmd.CombinedOutput(); err != nil {
			return prevBranch, fmt.Errorf("create orphan branch: %w", err)
		}
		// Remove cached index
		c := exec.Command("git", "rm", "-rf", "--cached", ".")
		c.Dir = repoPath
		c.Run()
		// Initial empty commit to establish branch
		c = exec.Command("git", "commit", "--allow-empty", "-m", "Selo: init receipt-anchor branch")
		c.Dir = repoPath
		c.Run()
	} else {
		cmd = exec.Command("git", "checkout", "-f", "receipt-anchor")
		cmd.Dir = repoPath
		if _, err := cmd.CombinedOutput(); err != nil {
			return prevBranch, fmt.Errorf("checkout receipt-anchor: %w", err)
		}
	}
	return prevBranch, nil
}

func restoreBranch(repoPath, branch string) {
	if branch == "" {
		return
	}
	cmd := exec.Command("git", "checkout", "-f", branch)
	cmd.Dir = repoPath
	cmd.CombinedOutput()
}

// AnchorReceipt creates an external anchor for a receipt by committing
// the receipt hash to a signed Git commit on the receipt-anchor branch.
// This provides external tamper-evidence that cannot be rewritten without detection.
func AnchorReceipt(receiptPath string, repoPath string) (*AnchorResult, error) {
	// Read the receipt file
	receiptData, err := os.ReadFile(receiptPath)
	if err != nil {
		return nil, fmt.Errorf("read receipt: %w", err)
	}

	// Compute receipt hash — prefer receipt_hash field (canonical, signed) if present
	hashStr := ""
	var parsed map[string]interface{}
	if err := json.Unmarshal(receiptData, &parsed); err == nil {
		if v, ok := parsed["receipt_hash"].(string); ok && v != "" {
			hashStr = v
		}
	}
	if hashStr == "" {
		receiptHash := sha256.Sum256(receiptData)
		hashStr = hex.EncodeToString(receiptHash[:])
	}

	// Create anchor file
	anchorDir := filepath.Join(repoPath, ".selo", "anchors")
	os.MkdirAll(anchorDir, 0755)

	anchorFile := filepath.Join(anchorDir, fmt.Sprintf("anchor-%s.txt", hashStr[:12]))
	anchorContent := fmt.Sprintf("Selo Receipt Anchor\nReceipt: %s\nHash: %s\nAnchored: %s\n",
		filepath.Base(receiptPath), hashStr, time.Now().UTC().Format(time.RFC3339))

	if err := os.WriteFile(anchorFile, []byte(anchorContent), 0644); err != nil {
		return nil, fmt.Errorf("write anchor file: %w", err)
	}

	// Checkout receipt-anchor branch
	prevBranch, _ := ensureReceiptAnchorBranch(repoPath)
	defer restoreBranch(repoPath, prevBranch)

	// Create signed Git commit
	commitHash, branch, err := createSignedCommit(repoPath, anchorFile, hashStr)
	if err != nil {
		// Fall back to unsigned commit
		commitHash, branch, err = createUnsignedCommit(repoPath, anchorFile, hashStr)
		if err != nil {
			return nil, fmt.Errorf("create commit: %w", err)
		}
	}

	return &AnchorResult{
		ReceiptHash: hashStr,
		AnchorHash:  commitHash,
		AnchorType:  "git_commit",
		AnchoredAt:  time.Now().UTC(),
		GitCommit:   commitHash,
		GitBranch:   branch,
	}, nil
}

// createSignedCommit attempts to create a GPG-signed Git commit.
func createSignedCommit(repoPath, anchorFile, receiptHash string) (string, string, error) {
	// Check if GPG is available
	if _, err := exec.LookPath("gpg"); err != nil {
		return "", "", fmt.Errorf("gpg not available: %w", err)
	}

	// Stage the anchor file
	cmd := exec.Command("git", "add", anchorFile)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git add: %s: %w", string(output), err)
	}

	// Create signed commit
	commitMsg := fmt.Sprintf("Selo: anchor receipt %s", receiptHash[:12])
	cmd = exec.Command("git", "commit", "-S", "-m", commitMsg)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	// Get commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git rev-parse: %w", err)
	}
	commitHash := strings.TrimSpace(string(output))

	// Get current branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))

	return commitHash, branch, nil
}

// createUnsignedCommit creates a regular Git commit (fallback).
func createUnsignedCommit(repoPath, anchorFile, receiptHash string) (string, string, error) {
	// Stage the anchor file
	cmd := exec.Command("git", "add", anchorFile)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git add: %s: %w", string(output), err)
	}

	// Create commit
	commitMsg := fmt.Sprintf("Selo: anchor receipt %s", receiptHash[:12])
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	// Get commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git rev-parse: %w", err)
	}
	commitHash := strings.TrimSpace(string(output))

	// Get current branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))

	return commitHash, branch, nil
}

// VerifyAnchor verifies that a receipt hash matches its anchor on receipt-anchor branch.
func VerifyAnchor(repoPath, receiptHash string) (bool, error) {
	anchorRel := fmt.Sprintf(".selo/anchors/anchor-%s.txt", receiptHash[:12])
	anchorFile := filepath.Join(repoPath, anchorRel)

	data, err := os.ReadFile(anchorFile)
	if err != nil {
		// Try git show from receipt-anchor branch
		cmd := exec.Command("git", "show", fmt.Sprintf("receipt-anchor:%s", anchorRel))
		cmd.Dir = repoPath
		if out, err2 := cmd.Output(); err2 == nil {
			data = out
		} else {
			return false, fmt.Errorf("read anchor: %w", err)
		}
	}

	// Verify the hash in the anchor file
	content := string(data)
	if !strings.Contains(content, receiptHash) {
		return false, fmt.Errorf("receipt hash mismatch in anchor")
	}

	// Verify the commit exists on receipt-anchor branch
	cmd := exec.Command("git", "log", "receipt-anchor", "--oneline", "--grep", fmt.Sprintf("anchor receipt %s", receiptHash[:12]))
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// Fallback to any branch
		cmd = exec.Command("git", "log", "--oneline", "--grep", fmt.Sprintf("anchor receipt %s", receiptHash[:12]))
		cmd.Dir = repoPath
		output, err = cmd.Output()
		if err != nil {
			return false, fmt.Errorf("git log: %w", err)
		}
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// BatchAnchorReceipts anchors multiple receipts in a single commit.
func BatchAnchorReceipts(receiptPaths []string, repoPath string) (*AnchorResult, error) {
	if len(receiptPaths) == 0 {
		return nil, fmt.Errorf("no receipts to anchor")
	}

	// Create anchor directory
	anchorDir := filepath.Join(repoPath, ".selo", "anchors")
	os.MkdirAll(anchorDir, 0755)

	var hashes []string
	for _, receiptPath := range receiptPaths {
		receiptData, err := os.ReadFile(receiptPath)
		if err != nil {
			return nil, fmt.Errorf("read receipt %s: %w", receiptPath, err)
		}

		hash := sha256.Sum256(receiptData)
		hashStr := hex.EncodeToString(hash[:])
		hashes = append(hashes, hashStr)

		// Create individual anchor file
		anchorFile := filepath.Join(anchorDir, fmt.Sprintf("anchor-%s.txt", hashStr[:12]))
		anchorContent := fmt.Sprintf("Selo Receipt Anchor\nReceipt: %s\nHash: %s\nAnchored: %s\n",
			filepath.Base(receiptPath), hashStr, time.Now().UTC().Format(time.RFC3339))

		if err := os.WriteFile(anchorFile, []byte(anchorContent), 0644); err != nil {
			return nil, fmt.Errorf("write anchor file: %w", err)
		}
	}

	// Checkout receipt-anchor branch
	prevBranch, _ := ensureReceiptAnchorBranch(repoPath)
	defer restoreBranch(repoPath, prevBranch)

	// Stage all anchor files
	cmd := exec.Command("git", "add", anchorDir)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git add: %s: %w", string(output), err)
	}

	// Create commit
	commitMsg := fmt.Sprintf("Selo: batch anchor %d receipts", len(receiptPaths))
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	// Get commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-parse: %w", err)
	}
	commitHash := strings.TrimSpace(string(output))

	// Get current branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))

	// Compute combined hash
	combined := strings.Join(hashes, ",")
	combinedHash := sha256.Sum256([]byte(combined))

	return &AnchorResult{
		ReceiptHash: hex.EncodeToString(combinedHash[:]),
		AnchorHash:  commitHash,
		AnchorType:  "git_commit",
		AnchoredAt:  time.Now().UTC(),
		GitCommit:   commitHash,
		GitBranch:   branch,
	}, nil
}

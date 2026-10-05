package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/spf13/cobra"
)

var (
	verifyRepoPath   string
	verifyWantAnchor bool
	verifyJSONOut    bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify <receipt.json>",
	Short: "Verify a signed receipt (content hash, signature, optional anchor)",
	Args:  cobra.ExactArgs(1),
	RunE:  runVerifyCmd,
}

func init() {
	verifyCmd.Flags().StringVar(&verifyRepoPath, "repo", ".", "Repo path for anchor verification")
	verifyCmd.Flags().BoolVar(&verifyWantAnchor, "anchor", false, "Also verify the git anchor")
	verifyCmd.Flags().BoolVar(&verifyJSONOut, "json", false, "Output machine-readable JSON")
}

// verifyResult is the outcome of verifying a receipt file.
// States rendered in text mode are tracked alongside the JSON fields.
type verifyResult struct {
	Valid       bool     `json:"valid"`
	ReceiptID   string   `json:"receipt_id"`
	HashOK      bool     `json:"hash_ok"`
	SignatureOK bool     `json:"signature_ok"`
	AnchorOK    *bool    `json:"anchor_ok"`
	Errors      []string `json:"errors"`

	Verdict        string `json:"-"`
	HashState      string `json:"-"` // OK | MISMATCH | MISSING
	SignatureState string `json:"-"` // OK | INVALID | UNSIGNED
	AnchorState    string `json:"-"` // OK | FAILED | SKIPPED
	Path           string `json:"-"` // file the result was computed from
}

// ReceiptPathHint returns the path third parties should pass to `selo verify`
// to reproduce this result.
func (res *verifyResult) ReceiptPathHint() string {
	return res.Path
}

// verifyReceiptFile loads and verifies a receipt. It never returns an error:
// I/O and parse failures are reported as an invalid result with a reason,
// so the caller can print the output and exit non-zero.
func verifyReceiptFile(path, repoPath string, wantAnchor bool) *verifyResult {
	res := &verifyResult{
		Errors:         []string{},
		HashState:      "MISSING",
		SignatureState: "INVALID",
		AnchorState:    "SKIPPED",
		Path:           path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("unreadable receipt: %v", err))
		return res
	}

	var r receipt.ForgeReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		res.ReceiptID = r.ReceiptID
		res.Verdict = r.Verdict
		res.Errors = append(res.Errors, fmt.Sprintf("malformed receipt JSON: %v", err))
		return res
	}
	res.ReceiptID = r.ReceiptID
	res.Verdict = r.Verdict

	// 1. Content hash: sha256 of canonical JSON must equal ReceiptHash.
	hashOK := false
	if r.ReceiptHash == "" {
		res.Errors = append(res.Errors, "receipt has no content hash")
	} else if canonical, cerr := receipt.CanonicalJSON(&r); cerr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("canonical json: %v", cerr))
	} else {
		sum := sha256.Sum256(canonical)
		if hex.EncodeToString(sum[:]) == r.ReceiptHash {
			hashOK = true
			res.HashState = "OK"
		} else {
			res.HashState = "MISMATCH"
			res.Errors = append(res.Errors, "content hash mismatch")
		}
	}
	res.HashOK = hashOK

	// 2. Signature: ed25519 over the canonical JSON.
	sigOK := false
	if ok, verr := receipt.VerifyReceipt(&r); verr == nil && ok {
		sigOK = true
		res.SignatureState = "OK"
	} else if r.Signature == "" || r.PublicKey == "" {
		res.SignatureState = "UNSIGNED"
		res.Errors = append(res.Errors, "receipt is unsigned")
	} else if verr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("signature verification failed: %v", verr))
	} else {
		res.SignatureState = "INVALID"
		res.Errors = append(res.Errors, "signature mismatch")
	}
	res.SignatureOK = sigOK

	// 3. Anchor: only checked when requested; otherwise skipped and ignored.
	if wantAnchor {
		anchorFail := func(reason string) {
			f := false
			res.AnchorOK = &f
			res.AnchorState = "FAILED"
			res.Errors = append(res.Errors, reason)
		}
		switch {
		case r.ReceiptHash == "":
			anchorFail("anchor verification failed: receipt has no content hash")
		case len(r.ReceiptHash) < 12:
			anchorFail("anchor verification failed: receipt hash too short")
		default:
			ok, aerr := receipt.VerifyAnchor(repoPath, r.ReceiptHash)
			if aerr != nil {
				anchorFail(fmt.Sprintf("anchor verification failed: %v", aerr))
			} else if ok {
				res.AnchorOK = &ok
				res.AnchorState = "OK"
			} else {
				anchorFail("anchor verification failed")
			}
		}
	}

	res.Valid = hashOK && sigOK && (res.AnchorOK == nil || *res.AnchorOK)
	return res
}

func printVerifyResult(res *verifyResult) {
	fmt.Printf("Receipt:    %s\n", res.ReceiptID)
	fmt.Printf("Verdict:    %s\n", res.Verdict)
	fmt.Printf("Hash:       %s\n", res.HashState)
	fmt.Printf("Signature:  %s\n", res.SignatureState)
	fmt.Printf("Anchor:     %s\n", res.AnchorState)
	fmt.Printf("Result:     %s\n", map[bool]string{true: "VALID", false: "INVALID"}[res.Valid])
	for _, e := range res.Errors {
		fmt.Printf("  - %s\n", e)
	}
}

func runVerifyCmd(cmd *cobra.Command, args []string) error {
	res := verifyReceiptFile(args[0], verifyRepoPath, verifyWantAnchor)

	if verifyJSONOut {
		out, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding verify result: %w", err)
		}
		fmt.Println(string(out))
	} else {
		printVerifyResult(res)
	}

	if !res.Valid {
		os.Exit(1)
	}
	return nil
}

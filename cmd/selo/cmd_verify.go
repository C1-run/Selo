package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/spf13/cobra"
)

var (
	verifyRepoPath   string
	verifyWantAnchor bool
	verifyJSONOut    bool
	verifyPubKey     string
)

var verifyCmd = &cobra.Command{
	Use:   "verify <receipt.json>",
	Short: "Verify a signed receipt (content hash, signature, optional anchor, pinned signer)",
	Args:  cobra.ExactArgs(1),
	RunE:  runVerifyCmd,
}

func init() {
	verifyCmd.Flags().StringVar(&verifyRepoPath, "repo", ".", "Repo path for anchor verification")
	verifyCmd.Flags().BoolVar(&verifyWantAnchor, "anchor", false, "Also verify the git anchor")
	verifyCmd.Flags().BoolVar(&verifyJSONOut, "json", false, "Output machine-readable JSON")
	verifyCmd.Flags().StringVar(&verifyPubKey, "pubkey", "", "Pin the signer: a path to a key file, a 64-char hex fingerprint, or an inline base64 public key. Without it, Selo only confirms internal self-consistency and cannot prove who signed.")
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

	KeyMode      string `json:"key_mode,omitempty"` // persistent | ephemeral (from receipt)
	Provenance   string `json:"provenance"`         // PINNED | UNPINNED_PERSISTENT | UNPINNED_EPHEMERAL | UNPINNED_UNKNOWN
	PubKeyPinned bool   `json:"pubkey_pinned"`
	PubKeyMatch  *bool  `json:"pubkey_match,omitempty"` // nil when no pin was given
}

// ReceiptPathHint returns the path third parties should pass to `selo verify`
// to reproduce this result.
func (res *verifyResult) ReceiptPathHint() string {
	return res.Path
}

// isHex reports whether s is a non-empty hex string.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// resolvePinnedFingerprint turns a --pubkey argument into the hex fingerprint
// that signer-pinning compares against. The argument may be a path to a key
// file (base64 public key or 64-char hex fingerprint) or an inline value.
func resolvePinnedFingerprint(arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", fmt.Errorf("empty --pubkey")
	}
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		data, rerr := os.ReadFile(arg)
		if rerr != nil {
			return "", fmt.Errorf("read --pubkey file: %w", rerr)
		}
		content := strings.TrimSpace(string(data))
		if fp, ferr := receipt.PublicKeyFingerprint(content); ferr == nil {
			return fp, nil
		}
		if len(content) == 64 && isHex(content) {
			return content, nil
		}
		return "", fmt.Errorf("--pubkey file is neither a base64 public key nor a 64-char hex fingerprint")
	}
	if len(arg) == 64 && isHex(arg) {
		return arg, nil
	}
	if fp, ferr := receipt.PublicKeyFingerprint(arg); ferr == nil {
		return fp, nil
	}
	return "", fmt.Errorf("--pubkey must be a path to a key file, a 64-char hex fingerprint, or an inline base64 public key")
}

// verifyReceiptFile loads and verifies a receipt. It never returns an error:
// I/O and parse failures are reported as an invalid result with a reason,
// so the caller can print the output and exit non-zero.
func verifyReceiptFile(path, repoPath string, wantAnchor bool, pinnedPubKey string) *verifyResult {
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
	res.KeyMode = r.KeyMode

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

	// 3. Signer pinning (the actual provenance guarantee).
	//    Without a pinned key, Selo can only confirm that the receipt is
	//    internally self-consistent — it cannot prove WHO signed it. A
	//    forged receipt signed by an attacker's own key verifies just as well
	//    as a genuine one until a trusted fingerprint is supplied.
	res.Provenance = "UNPINNED_UNKNOWN"
	if pinnedPubKey != "" {
		res.PubKeyPinned = true
		pinnedFP, perr := resolvePinnedFingerprint(pinnedPubKey)
		fp, fperr := receipt.PublicKeyFingerprint(r.PublicKey)
		match := false
		if perr == nil && fperr == nil && pinnedFP == fp {
			match = true
		}
		res.PubKeyMatch = &match
		if perr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("invalid --pubkey: %v", perr))
		} else if fperr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("receipt public key: %v", fperr))
		} else if !match {
			res.Errors = append(res.Errors, "signer fingerprint does not match pinned key (receipt not from the trusted signer)")
		}
		res.Provenance = "PINNED"
	} else {
		switch r.KeyMode {
		case receipt.KeyModePersistent:
			res.Provenance = "UNPINNED_PERSISTENT"
		case receipt.KeyModeEphemeral:
			res.Provenance = "UNPINNED_EPHEMERAL"
		default:
			res.Provenance = "UNPINNED_UNKNOWN"
		}
	}

	// 4. Anchor: only checked when requested; otherwise skipped and ignored.
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

	// Overall validity: every check must pass. A pinned-key mismatch forces
	// INVALID even when hash + signature are internally consistent.
	pinOK := res.PubKeyMatch == nil || *res.PubKeyMatch
	res.Valid = hashOK && sigOK && pinOK && (res.AnchorOK == nil || *res.AnchorOK)
	return res
}

func printVerifyResult(res *verifyResult) {
	fmt.Printf("Receipt:    %s\n", res.ReceiptID)
	fmt.Printf("Verdict:    %s\n", res.Verdict)
	fmt.Printf("Hash:       %s\n", res.HashState)
	fmt.Printf("Signature:  %s\n", res.SignatureState)
	fmt.Printf("Anchor:     %s\n", res.AnchorState)
	fmt.Printf("Key mode:   %s\n", keyModeLabel(res.KeyMode))
	fmt.Printf("Provenance: %s\n", res.Provenance)
	if !res.PubKeyPinned {
		fmt.Printf("⚠️  Provenance NOT pinned: this receipt is internally consistent but Selo cannot prove who signed it. Re-run with --pubkey <fingerprint|file> to confirm the signer.\n")
	}
	fmt.Printf("Result:     %s\n", map[bool]string{true: "VALID", false: "INVALID"}[res.Valid])
	for _, e := range res.Errors {
		fmt.Printf("  - %s\n", e)
	}
}

func keyModeLabel(mode string) string {
	switch mode {
	case receipt.KeyModePersistent:
		return "persistent"
	case receipt.KeyModeEphemeral:
		return "ephemeral (per-process, not attributable across runs)"
	default:
		return "unknown (legacy receipt)"
	}
}

func runVerifyCmd(cmd *cobra.Command, args []string) error {
	res := verifyReceiptFile(args[0], verifyRepoPath, verifyWantAnchor, verifyPubKey)

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

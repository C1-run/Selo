package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/spf13/cobra"
)

var (
	verifyRepoPath   string
	verifyWantAnchor bool
	verifyJSONOut    bool
	verifyPubKey     string
	verifyTSACA      string
	verifyRequireTSA bool
	verifyRekorKey   string
	verifyRequireRek bool
	// verifyRequireKeySource, when set, fails the receipt unless its signing key
	// came from the named source. Only "command" (ADR-008) places the signer
	// outside the audited agent's trust domain. key_source is self-asserted, so
	// the gate only means something together with --pubkey; the two are enforced
	// as a pair (see verifyReceiptStruct).
	verifyRequireKeySource string
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
	verifyCmd.Flags().StringVar(&verifyTSACA, "tsa-ca", "", "PEM file of trusted TSA root certificate(s) to anchor an RFC3161 timestamp to a trusted TSA (ADR-005)")
	verifyCmd.Flags().BoolVar(&verifyRequireTSA, "require-tsa", false, "Fail unless the receipt carries a trusted timestamp verified against --tsa-ca")
	verifyCmd.Flags().StringVar(&verifyRekorKey, "rekor-pubkey", "", "PEM file of the Rekor log's public key, to verify the transparency-log checkpoint signature (ADR-006)")
	verifyCmd.Flags().BoolVar(&verifyRequireRek, "require-rekor", false, "Fail unless the receipt carries a transparency-log entry verified against --rekor-pubkey")
	verifyCmd.Flags().StringVar(&verifyRequireKeySource, "require-keysource", "", "Fail unless the receipt's signing key came from the named source (e.g. 'command'). Only 'command' places the signer outside the audited agent's trust domain (ADR-008). key_source is self-asserted, so this must be paired with --pubkey to mean anything.")
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

	KeyMode      string `json:"key_mode,omitempty"`   // persistent | ephemeral (from receipt)
	KeySource    string `json:"key_source,omitempty"` // env | file | keychain | command | ephemeral
	Provenance   string `json:"provenance"`           // PINNED | UNPINNED_PERSISTENT | UNPINNED_EPHEMERAL | UNPINNED_UNKNOWN
	PubKeyPinned bool   `json:"pubkey_pinned"`
	PubKeyMatch  *bool  `json:"pubkey_match,omitempty"` // nil when no pin was given
	Format       string `json:"format,omitempty"`       // receipt | in-toto/DSSE

	// Signer trust domain (ADR-008). KeySourceOK is nil unless a
	// --require-keysource gate was set and we evaluated it.
	KeySourceOK     *bool  `json:"key_source_ok,omitempty"`
	KeySourceState  string `json:"key_source_state,omitempty"` // OK | UNVERIFIED | FAILED | ABSENT | SKIPPED
	KeySourceReason string `json:"key_source_reason,omitempty"`

	// Trusted timestamp (ADR-005). TimestampOK is nil unless the receipt
	// carries a timestamp we actually evaluated.
	TimestampOK     *bool  `json:"timestamp_ok,omitempty"`
	TimestampState  string `json:"timestamp_state,omitempty"` // OK | UNVERIFIED | FAILED | ABSENT | SKIPPED
	TimestampTSA    string `json:"timestamp_tsa,omitempty"`
	TimestampTime   string `json:"timestamp_time,omitempty"`
	TimestampReason string `json:"timestamp_reason,omitempty"`

	// Transparency log (ADR-006). TransparencyOK is nil unless the receipt
	// carries a log record we actually evaluated.
	TransparencyOK     *bool  `json:"transparency_ok,omitempty"`
	TransparencyState  string `json:"transparency_state,omitempty"` // OK | UNVERIFIED | FAILED | ABSENT | SKIPPED
	TransparencyLog    string `json:"transparency_log,omitempty"`
	TransparencyIndex  int64  `json:"transparency_index,omitempty"`
	TransparencyTime   string `json:"transparency_time,omitempty"`
	TransparencyReason string `json:"transparency_reason,omitempty"`
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

// pinnedKeyMaterial returns the base64 public key when --pubkey carries actual
// key material (a key file or an inline base64 key). A bare 64-char hex
// fingerprint is not key material — it can only pin, not verify.
func pinnedKeyMaterial(arg string) (string, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", false
	}
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		data, rerr := os.ReadFile(arg)
		if rerr != nil {
			return "", false
		}
		content := strings.TrimSpace(string(data))
		if _, ferr := receipt.PublicKeyFingerprint(content); ferr == nil {
			return content, true
		}
		return "", false
	}
	if len(arg) == 64 && isHex(arg) {
		return "", false
	}
	if _, ferr := receipt.PublicKeyFingerprint(arg); ferr == nil {
		return arg, true
	}
	return "", false
}

// isDSSEEnvelope reports whether the bytes are a DSSE envelope (ADR-001) rather
// than a native receipt.
func isDSSEEnvelope(data []byte) bool {
	var probe struct {
		PayloadType string `json:"payloadType"`
		Payload     string `json:"payload"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return false
	}
	return probe.PayloadType != "" && probe.Payload != ""
}

// verifyReceiptFile loads and verifies a receipt or an in-toto/DSSE attestation.
// It never returns an error: I/O and parse failures are reported as an invalid
// result with a reason, so the caller can print the output and exit non-zero.
func verifyReceiptFile(path, repoPath string, wantAnchor bool, pinnedPubKey string) *verifyResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return &verifyResult{
			Errors:         []string{fmt.Sprintf("unreadable receipt: %v", err)},
			HashState:      "MISSING",
			SignatureState: "INVALID",
			AnchorState:    "SKIPPED",
			Path:           path,
			Format:         "receipt",
		}
	}
	if isDSSEEnvelope(data) {
		return verifyEnvelope(data, path, repoPath, wantAnchor, pinnedPubKey)
	}

	res := &verifyResult{
		Errors:         []string{},
		HashState:      "MISSING",
		SignatureState: "INVALID",
		AnchorState:    "SKIPPED",
		Path:           path,
		Format:         "receipt",
	}
	var r receipt.ForgeReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		res.ReceiptID = r.ReceiptID
		res.Verdict = r.Verdict
		res.Errors = append(res.Errors, fmt.Sprintf("malformed receipt JSON: %v", err))
		return res
	}
	verifyReceiptStruct(&r, res, repoPath, wantAnchor, pinnedPubKey)
	finalizeResult(res, true)
	return res
}

// verifyEnvelope verifies a DSSE-wrapped in-toto attestation: the envelope
// signature over the PAE, then the inner receipt's own hash and signature.
func verifyEnvelope(data []byte, path, repoPath string, wantAnchor bool, pinnedPubKey string) *verifyResult {
	res := &verifyResult{
		Errors:         []string{},
		HashState:      "MISSING",
		SignatureState: "INVALID",
		AnchorState:    "SKIPPED",
		Path:           path,
		Format:         "in-toto/DSSE",
	}
	var env receipt.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("malformed DSSE envelope: %v", err))
		return res
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("envelope payload: %v", err))
		return res
	}
	var stmt receipt.Statement
	if err := json.Unmarshal(payload, &stmt); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("malformed in-toto statement: %v", err))
		return res
	}
	var r receipt.ForgeReceipt
	if len(stmt.Predicate) > 0 {
		if err := json.Unmarshal(stmt.Predicate, &r); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("malformed predicate: %v", err))
			return res
		}
	}
	res.ReceiptID = r.ReceiptID
	res.Verdict = r.Verdict
	res.KeyMode = r.KeyMode
	res.KeySource = r.KeySource

	// Envelope signature. Verify against key material from --pubkey when given;
	// otherwise fall back to the predicate's embedded key, which proves
	// self-consistency only (the same caveat as an unpinned receipt).
	dsseOK := false
	signerKey := ""
	if k, ok := pinnedKeyMaterial(pinnedPubKey); ok {
		signerKey = k
	} else if r.PublicKey != "" {
		signerKey = r.PublicKey
	}
	if signerKey == "" {
		res.Errors = append(res.Errors, "no public key available to verify the envelope (pass --pubkey <key-file>)")
	} else {
		_, ok, verr := receipt.VerifyStatement(&env, signerKey)
		if verr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("envelope: %v", verr))
		} else if ok {
			dsseOK = true
		} else {
			res.Errors = append(res.Errors, "DSSE signature invalid")
		}
		// The envelope's declared keyid must match the key we verified with.
		if len(env.Signatures) > 0 && env.Signatures[0].KeyID != "" {
			if fp, ferr := receipt.PublicKeyFingerprint(signerKey); ferr == nil && fp != env.Signatures[0].KeyID {
				res.Errors = append(res.Errors, "envelope keyid does not match the signing key")
				dsseOK = false
			}
		}
	}

	// Inner receipt: content hash + signature + pinning + anchor + trust domain.
	verifyReceiptStruct(&r, res, repoPath, wantAnchor, pinnedPubKey)
	finalizeResult(res, dsseOK)
	return res
}

// verifyReceiptStruct runs the receipt-level checks (content hash, signature,
// signer pinning, optional anchor, optional trust-domain gate) and fills res.
// It does not set res.Valid.
func verifyReceiptStruct(r *receipt.ForgeReceipt, res *verifyResult, repoPath string, wantAnchor bool, pinnedPubKey string) {
	res.ReceiptID = r.ReceiptID
	res.Verdict = r.Verdict
	res.KeyMode = r.KeyMode
	res.KeySource = r.KeySource

	// 1. Content hash: sha256 of canonical JSON must equal ReceiptHash.
	hashOK := false
	if r.ReceiptHash == "" {
		res.Errors = append(res.Errors, "receipt has no content hash")
	} else if canonical, cerr := receipt.CanonicalJSON(r); cerr != nil {
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
	if ok, verr := receipt.VerifyReceipt(r); verr == nil && ok {
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

	// 3b. Signer trust domain (ADR-008). The signing side can keep the key out
	// of the agent's process via the 'command' backend; this gate makes that
	// choice checkable. A receipt signed by a key the agent itself could read
	// (env | file | keychain | ephemeral) cannot satisfy
	// --require-keysource=command. key_source is self-asserted, so the gate is
	// enforced together with --pubkey: without a pinned key a receipt that
	// simply labels itself "command" would pass. Without the gate the source is
	// reported but not required.
	res.KeySourceState = receipt.KeySourceStateSkipped
	switch {
	case verifyRequireKeySource != "" && pinnedPubKey == "":
		// Fail closed. key_source is self-asserted, so without a pinned key the
		// gate cannot tell a real external signer from a receipt that merely
		// claims one: anyone holding a signing key can sign a receipt labelled
		// "command" and it would pass. Require the pin rather than return a
		// result that looks gated but is not.
		f := false
		res.KeySourceOK = &f
		res.KeySourceState = receipt.KeySourceStateFailed
		res.Errors = append(res.Errors, fmt.Sprintf("--require-keysource=%s needs --pubkey: key_source is self-asserted, so the gate alone cannot detect a falsely labelled signer", verifyRequireKeySource))
	case verifyRequireKeySource == "" && r.KeySource == "":
		// No gate was requested, so a missing source is reported, not fatal.
		res.KeySourceState = receipt.KeySourceStateAbsent
	case verifyRequireKeySource == "":
		res.KeySourceState = receipt.KeySourceStateUnverified
		res.KeySourceReason = "no --require-keysource given: the source is reported but not gated"
	case r.KeySource == "":
		// Fail closed. A gate is set but the receipt records no source, so
		// nothing shows the signer was external. Passing here would make the
		// gate bypassable by clearing one field: an attacker holding the
		// signing key (which is the very threat this gate exists for) could
		// re-sign the receipt with key_source omitted.
		f := false
		res.KeySourceOK = &f
		res.KeySourceState = receipt.KeySourceStateAbsent
		res.Errors = append(res.Errors, fmt.Sprintf("--require-keysource=%s: receipt records no key_source, so the signer cannot be shown to sit outside the agent's trust domain; a missing field is not compliance", verifyRequireKeySource))
	case r.KeySource == verifyRequireKeySource:
		ok := true
		res.KeySourceOK = &ok
		res.KeySourceState = receipt.KeySourceStateOK
	default:
		f := false
		res.KeySourceOK = &f
		res.KeySourceState = receipt.KeySourceStateFailed
		res.Errors = append(res.Errors, fmt.Sprintf("--require-keysource=%s: receipt was signed by key source %q, which shares the agent's trust domain", verifyRequireKeySource, r.KeySource))
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

	// 5. Trusted timestamp (ADR-005). Absence is reported but not fatal unless
	// the caller required one; a present-but-unverifiable token is an error.
	res.TimestampState = receipt.TimestampStateSkipped
	if r.Timestamp == nil {
		res.TimestampState = receipt.TimestampStateAbsent
	} else {
		res.TimestampTSA = r.Timestamp.TSAURL
		if !r.Timestamp.GenTime.IsZero() {
			res.TimestampTime = r.Timestamp.GenTime.UTC().Format(time.RFC3339)
		}
		switch {
		case r.Timestamp.Status == receipt.TimestampAbsent:
			res.TimestampState = receipt.TimestampStateAbsent
			res.TimestampReason = r.Timestamp.Reason
		case r.ReceiptHash == "":
			f := false
			res.TimestampOK = &f
			res.TimestampState = receipt.TimestampStateFailed
			res.Errors = append(res.Errors, "timestamp cannot be checked: receipt has no content hash")
		default:
			var roots *x509.CertPool
			if verifyTSACA != "" {
				p, perr := receipt.LoadTSARoots(verifyTSACA)
				if perr != nil {
					f := false
					res.TimestampOK = &f
					res.TimestampState = receipt.TimestampStateFailed
					res.Errors = append(res.Errors, fmt.Sprintf("load --tsa-ca: %v", perr))
					break
				}
				roots = p
			}
			if terr := receipt.VerifyTimestamp(r.Timestamp, r.ReceiptHash, roots); terr != nil {
				f := false
				res.TimestampOK = &f
				res.TimestampState = receipt.TimestampStateFailed
				res.Errors = append(res.Errors, fmt.Sprintf("timestamp: %v", terr))
			} else if roots == nil {
				res.TimestampState = receipt.TimestampStateUnverified
				res.TimestampReason = "no --tsa-ca given: the token is self-consistent but not anchored to a trusted TSA"
			} else {
				ok := true
				res.TimestampOK = &ok
				res.TimestampState = receipt.TimestampStateOK
			}
		}
	}
	// --require-tsa turns any timestamp short of verified-against-a-trusted-TSA
	// into a failure, so a caller can gate on L3 instead of reading prose.
	if verifyRequireTSA && res.TimestampState != receipt.TimestampStateOK {
		f := false
		res.TimestampOK = &f
		res.TimestampState = receipt.TimestampStateFailed
		res.Errors = append(res.Errors, "--require-tsa: receipt has no trusted, verified timestamp")
	}

	// 6. Transparency log (ADR-006). Same shape as the timestamp: absence is
	// reported, an unverifiable record is an error.
	res.TransparencyState = receipt.TransparencyStateSkipped
	if r.Transparency == nil {
		res.TransparencyState = receipt.TransparencyStateAbsent
	} else {
		res.TransparencyLog = r.Transparency.LogURL
		res.TransparencyIndex = r.Transparency.LogIndex
		if r.Transparency.IntegratedTime > 0 {
			res.TransparencyTime = time.Unix(r.Transparency.IntegratedTime, 0).UTC().Format(time.RFC3339)
		}
		switch {
		case r.Transparency.Status == receipt.TransparencyAbsent:
			res.TransparencyState = receipt.TransparencyStateAbsent
			res.TransparencyReason = r.Transparency.Reason
		default:
			var logKey string
			if verifyRekorKey != "" {
				k, kerr := receipt.LoadRekorPublicKey(verifyRekorKey)
				if kerr != nil {
					f := false
					res.TransparencyOK = &f
					res.TransparencyState = receipt.TransparencyStateFailed
					res.Errors = append(res.Errors, fmt.Sprintf("load --rekor-pubkey: %v", kerr))
					break
				}
				logKey = k
			}
			if terr := receipt.VerifyTransparency(r.Transparency, r.ReceiptHash, logKey); terr != nil {
				f := false
				res.TransparencyOK = &f
				res.TransparencyState = receipt.TransparencyStateFailed
				res.Errors = append(res.Errors, fmt.Sprintf("transparency: %v", terr))
			} else if logKey == "" {
				res.TransparencyState = receipt.TransparencyStateUnverified
				res.TransparencyReason = "no --rekor-pubkey given: the inclusion proof is consistent but not anchored to a trusted log key"
			} else {
				ok := true
				res.TransparencyOK = &ok
				res.TransparencyState = receipt.TransparencyStateOK
			}
		}
	}
	if verifyRequireRek && res.TransparencyState != receipt.TransparencyStateOK {
		f := false
		res.TransparencyOK = &f
		res.TransparencyState = receipt.TransparencyStateFailed
		res.Errors = append(res.Errors, "--require-rekor: receipt has no verified transparency-log entry")
	}
}

// finalizeResult computes overall validity: every check must pass. extraOK lets
// the envelope path require a valid DSSE signature on top of the inner receipt.
func finalizeResult(res *verifyResult, extraOK bool) {
	pinOK := res.PubKeyMatch == nil || *res.PubKeyMatch
	tsOK := res.TimestampOK == nil || *res.TimestampOK
	trOK := res.TransparencyOK == nil || *res.TransparencyOK
	ksOK := res.KeySourceOK == nil || *res.KeySourceOK
	res.Valid = res.HashOK && res.SignatureOK && pinOK && extraOK && tsOK && trOK && ksOK &&
		(res.AnchorOK == nil || *res.AnchorOK)
}

func printVerifyResult(res *verifyResult) {
	fmt.Printf("Receipt:    %s\n", res.ReceiptID)
	fmt.Printf("Verdict:    %s\n", res.Verdict)
	if res.Format != "" {
		fmt.Printf("Format:     %s\n", res.Format)
	}
	fmt.Printf("Hash:       %s\n", res.HashState)
	fmt.Printf("Signature:  %s\n", res.SignatureState)
	fmt.Printf("Anchor:     %s\n", res.AnchorState)
	fmt.Printf("Timestamp:  %s\n", res.TimestampState)
	if res.TimestampTSA != "" {
		fmt.Printf("TSA:        %s\n", res.TimestampTSA)
	}
	if res.TimestampTime != "" {
		fmt.Printf("Timestamped: %s\n", res.TimestampTime)
	}
	if res.TimestampReason != "" {
		fmt.Printf("            %s\n", res.TimestampReason)
	}
	fmt.Printf("Transparency: %s\n", res.TransparencyState)
	if res.TransparencyLog != "" {
		fmt.Printf("Log:        %s\n", res.TransparencyLog)
	}
	if res.TransparencyIndex > 0 {
		fmt.Printf("Log index:  %d\n", res.TransparencyIndex)
	}
	if res.TransparencyTime != "" {
		fmt.Printf("Integrated: %s\n", res.TransparencyTime)
	}
	if res.TransparencyReason != "" {
		fmt.Printf("            %s\n", res.TransparencyReason)
	}
	fmt.Printf("Key mode:   %s\n", keyModeLabel(res.KeyMode))
	if res.KeySource != "" {
		fmt.Printf("Key source: %s\n", res.KeySource)
	}
	if verifyRequireKeySource != "" {
		fmt.Printf("Key-source gate (%s): %s\n", verifyRequireKeySource, res.KeySourceState)
		if res.KeySourceReason != "" {
			fmt.Printf("            %s\n", res.KeySourceReason)
		}
	}
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
	if verifyRequireKeySource != "" && verifyPubKey == "" {
		return fmt.Errorf("--require-keysource=%s requires --pubkey <key>: key_source is self-asserted, so the gate alone cannot detect a falsely labelled signer", verifyRequireKeySource)
	}
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

package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/C1-run/selo/internal/receipt"
)

func newVerifyTestReceipt() receipt.ForgeReceipt {
	now := time.Now()
	return receipt.ForgeReceipt{
		ReceiptID:  "r-1",
		TaskID:     "t-1",
		Verdict:    "PASS",
		StartedAt:  now,
		FinishedAt: now,
		ExitCode:   0,
	}
}

func writeVerifyTestReceipt(t *testing.T, r *receipt.ForgeReceipt) string {
	t.Helper()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	path := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	return path
}

func signedVerifyTestReceipt(t *testing.T) receipt.ForgeReceipt {
	t.Helper()
	seedB64, _, _, err := receipt.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	t.Setenv("SELO_SIGNING_KEY", seedB64)

	r := newVerifyTestReceipt()
	if _, err := receipt.SignReceipt(&r); err != nil {
		t.Fatalf("sign receipt: %v", err)
	}
	return r
}

// TestVerifyInTotoEnvelope covers the ADR-001 export path end to end: a receipt
// exported as a DSSE-wrapped in-toto statement must verify, and a tampered
// envelope must not.
func TestVerifyInTotoEnvelope(t *testing.T) {
	r := signedVerifyTestReceipt(t)

	env, err := receipt.ExportInToto(&r)
	if err != nil {
		t.Fatalf("ExportInToto: %v", err)
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "attestation.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Pinned with the signer's own key: valid, with proven provenance.
	res := verifyReceiptFile(path, t.TempDir(), false, r.PublicKey)
	if !res.Valid {
		t.Fatalf("expected VALID envelope, got INVALID: %v", res.Errors)
	}
	if res.Format != "in-toto/DSSE" {
		t.Fatalf("format = %q, want in-toto/DSSE", res.Format)
	}
	if res.Provenance != "PINNED" {
		t.Fatalf("provenance = %q, want PINNED", res.Provenance)
	}

	// Tamper with the payload: the envelope signature must fail.
	var tampered receipt.Envelope
	if err := json.Unmarshal(data, &tampered); err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(tampered.Payload)
	tampered.Payload = base64.StdEncoding.EncodeToString(append(raw, ' '))
	td, _ := json.MarshalIndent(&tampered, "", "  ")
	tpath := filepath.Join(t.TempDir(), "tampered.json")
	if err := os.WriteFile(tpath, td, 0644); err != nil {
		t.Fatal(err)
	}
	if bad := verifyReceiptFile(tpath, t.TempDir(), false, r.PublicKey); bad.Valid {
		t.Fatal("tampered envelope verified as VALID")
	}
}

func TestVerifyValidReceipt(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)

	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if !res.Valid {
		t.Fatalf("expected VALID, got INVALID: %v", res.Errors)
	}
	if !res.HashOK {
		t.Errorf("expected hash_ok, got false (state %s)", res.HashState)
	}
	if !res.SignatureOK {
		t.Errorf("expected signature_ok, got false (state %s)", res.SignatureState)
	}
	if res.AnchorOK != nil {
		t.Errorf("expected anchor_ok null (skipped), got %v", *res.AnchorOK)
	}
	if res.AnchorState != "SKIPPED" {
		t.Errorf("expected anchor state SKIPPED, got %s", res.AnchorState)
	}
	if res.ReceiptID != "r-1" {
		t.Errorf("expected receipt id r-1, got %q", res.ReceiptID)
	}
	if len(res.Errors) != 0 {
		t.Errorf("expected no errors, got %v", res.Errors)
	}
}

func TestVerifyTamperedReceipt(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	r.Verdict = "FAIL" // tamper after signing
	path := writeVerifyTestReceipt(t, &r)

	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if res.Valid {
		t.Fatal("expected INVALID for tampered receipt")
	}
	if res.HashOK {
		t.Error("expected hash_ok false")
	}
	if res.HashState != "MISMATCH" {
		t.Errorf("expected hash state MISMATCH, got %s", res.HashState)
	}
	if res.SignatureOK {
		t.Error("expected signature_ok false")
	}
	if res.SignatureState != "INVALID" {
		t.Errorf("expected signature state INVALID, got %s", res.SignatureState)
	}
	joined := strings.Join(res.Errors, "; ")
	if !strings.Contains(joined, "content hash mismatch") {
		t.Errorf("expected content hash mismatch error, got %v", res.Errors)
	}
	if !strings.Contains(joined, "signature mismatch") {
		t.Errorf("expected signature mismatch error, got %v", res.Errors)
	}
}

func TestVerifyUnsignedReceipt(t *testing.T) {
	r := newVerifyTestReceipt() // no SignReceipt call
	path := writeVerifyTestReceipt(t, &r)

	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if res.Valid {
		t.Fatal("expected INVALID for unsigned receipt")
	}
	if res.HashOK || res.SignatureOK {
		t.Errorf("expected hash/signature not ok, got hash=%v sig=%v", res.HashOK, res.SignatureOK)
	}
	if res.HashState != "MISSING" {
		t.Errorf("expected hash state MISSING, got %s", res.HashState)
	}
	if res.SignatureState != "UNSIGNED" {
		t.Errorf("expected signature state UNSIGNED, got %s", res.SignatureState)
	}
	joined := strings.Join(res.Errors, "; ")
	if !strings.Contains(joined, "receipt has no content hash") {
		t.Errorf("expected no content hash error, got %v", res.Errors)
	}
	if !strings.Contains(joined, "receipt is unsigned") {
		t.Errorf("expected unsigned error, got %v", res.Errors)
	}
}

func TestVerifyAnchorMissing(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)

	// Non-git temp dir: no anchor file and no receipt-anchor branch.
	res := verifyReceiptFile(path, t.TempDir(), true, "")
	if res.Valid {
		t.Fatal("expected INVALID when anchor requested but absent")
	}
	if !res.HashOK || !res.SignatureOK {
		t.Errorf("expected hash/signature ok, got hash=%v sig=%v", res.HashOK, res.SignatureOK)
	}
	if res.AnchorOK == nil || *res.AnchorOK {
		t.Errorf("expected anchor_ok false, got %v", res.AnchorOK)
	}
	if res.AnchorState != "FAILED" {
		t.Errorf("expected anchor state FAILED, got %s", res.AnchorState)
	}
	joined := strings.Join(res.Errors, "; ")
	if !strings.Contains(joined, "anchor verification failed") {
		t.Errorf("expected anchor failure error, got %v", res.Errors)
	}
}

func TestVerifyUnreadableReceipt(t *testing.T) {
	res := verifyReceiptFile(filepath.Join(t.TempDir(), "missing.json"), t.TempDir(), false, "")
	if res.Valid {
		t.Fatal("expected INVALID for unreadable receipt")
	}
	joined := strings.Join(res.Errors, "; ")
	if !strings.Contains(joined, "unreadable receipt") {
		t.Errorf("expected unreadable receipt error, got %v", res.Errors)
	}
}

func TestVerifyMalformedReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if res.Valid {
		t.Fatal("expected INVALID for malformed receipt")
	}
	joined := strings.Join(res.Errors, "; ")
	if !strings.Contains(joined, "malformed receipt JSON") {
		t.Errorf("expected malformed receipt JSON error, got %v", res.Errors)
	}
}

func TestVerifyPinnedKeyMatch(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)

	fp, err := receipt.PublicKeyFingerprint(r.PublicKey)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	res := verifyReceiptFile(path, t.TempDir(), false, fp)
	if !res.Valid {
		t.Fatalf("expected VALID with matching pinned key, got errors: %v", res.Errors)
	}
	if !res.PubKeyPinned {
		t.Error("expected pubkey_pinned true")
	}
	if res.PubKeyMatch == nil || !*res.PubKeyMatch {
		t.Error("expected pubkey_match true")
	}
	if res.Provenance != "PINNED" {
		t.Errorf("expected provenance PINNED, got %s", res.Provenance)
	}
}

func TestVerifyPinnedKeyMismatch(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)

	// A different keypair's fingerprint: the receipt cannot have been signed by it.
	otherSeed, _, _, err := receipt.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	t.Setenv("SELO_SIGNING_KEY", otherSeed)
	otherR := newVerifyTestReceipt()
	if _, err := receipt.SignReceipt(&otherR); err != nil {
		t.Fatalf("sign other: %v", err)
	}
	otherFP, err := receipt.PublicKeyFingerprint(otherR.PublicKey)
	if err != nil {
		t.Fatalf("fingerprint other: %v", err)
	}

	res := verifyReceiptFile(path, t.TempDir(), false, otherFP)
	if res.Valid {
		t.Fatal("expected INVALID when pinned key does not match the signer")
	}
	if res.PubKeyMatch == nil || *res.PubKeyMatch {
		t.Error("expected pubkey_match false")
	}
	joined := strings.Join(res.Errors, "; ")
	if !strings.Contains(joined, "does not match pinned key") {
		t.Errorf("expected pin mismatch error, got %v", res.Errors)
	}
}

func TestVerifyUnpinnedShowsProvenancePersistent(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)

	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if !res.Valid {
		t.Fatalf("expected internally-valid receipt, got errors: %v", res.Errors)
	}
	if res.PubKeyPinned {
		t.Error("expected pubkey_pinned false when no --pubkey given")
	}
	if res.Provenance != "UNPINNED_PERSISTENT" {
		t.Errorf("expected provenance UNPINNED_PERSISTENT, got %s", res.Provenance)
	}
	if r.KeyMode != receipt.KeyModePersistent {
		t.Errorf("expected receipt key_mode persistent, got %q", r.KeyMode)
	}
}

func TestVerifyEphemeralKeyModeUnpinned(t *testing.T) {
	// No persistent key: isolate HOME so a developer's real ~/.selo/signing-key
	// can't be auto-loaded, and allow ephemeral via env, which stamps
	// key_mode=ephemeral.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SELO_SIGNING_KEY", "")
	t.Setenv("SELO_ALLOW_EPHEMERAL_KEY", "1")
	r := newVerifyTestReceipt()
	if _, err := receipt.SignReceipt(&r); err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	if r.KeyMode != receipt.KeyModeEphemeral {
		t.Fatalf("expected key_mode ephemeral, got %q", r.KeyMode)
	}
	path := writeVerifyTestReceipt(t, &r)
	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if !res.Valid {
		t.Fatalf("expected internally-valid ephemeral receipt, got errors: %v", res.Errors)
	}
	if res.Provenance != "UNPINNED_EPHEMERAL" {
		t.Errorf("expected provenance UNPINNED_EPHEMERAL, got %s", res.Provenance)
	}
}

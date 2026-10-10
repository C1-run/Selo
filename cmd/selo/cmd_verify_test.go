package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

// fingerprintOf returns the hex fingerprint of a base64 ed25519 public key, the
// form --pubkey accepts.
func fingerprintOf(t *testing.T, pubB64 string) string {
	t.Helper()
	fp, err := receipt.PublicKeyFingerprint(pubB64)
	if err != nil {
		t.Fatalf("fingerprint %q: %v", pubB64, err)
	}
	return fp
}

// commandSourcedReceiptSimulated builds a receipt a real external signer would
// have produced: it is signed by a key Selo never loaded, and records
// KeySource=command. This exercises the verify gate without spawning a signer
// process (the real out-of-process path is covered separately below).
func commandSourcedReceiptSimulated(t *testing.T) receipt.ForgeReceipt {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	r := newVerifyTestReceipt()
	r.KeyMode = receipt.KeyModePersistent
	r.KeySource = receipt.KeySourceCommand
	canonical, err := receipt.CanonicalJSON(&r)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	sum := sha256.Sum256(canonical)
	r.ReceiptHash = hex.EncodeToString(sum[:])
	r.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical))
	r.PublicKey = base64.StdEncoding.EncodeToString(pub)
	return r
}

// buildReferenceSigner compiles scripts/selo-signer so a test can exercise the
// real ADR-008 command backend end to end.
func buildReferenceSigner(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "scripts", "selo-signer", "main.go")
	bin := filepath.Join(t.TempDir(), "selo-signer")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build reference signer: %v\n%s", err, out)
	}
	return bin
}

// setVerifyFlags sets the package-level verify flags for one test and restores
// them afterwards.
func setVerifyFlags(t *testing.T, tsaCA string, requireTSA bool, rekorKey string, requireRekor bool, requireKeySource string) {
	t.Helper()
	prevCA, prevRTSA, prevKey, prevRRek, prevRKS := verifyTSACA, verifyRequireTSA, verifyRekorKey, verifyRequireRek, verifyRequireKeySource
	verifyTSACA, verifyRequireTSA, verifyRekorKey, verifyRequireRek, verifyRequireKeySource = tsaCA, requireTSA, rekorKey, requireRekor, requireKeySource
	t.Cleanup(func() {
		verifyTSACA, verifyRequireTSA, verifyRekorKey, verifyRequireRek, verifyRequireKeySource = prevCA, prevRTSA, prevKey, prevRRek, prevRKS
	})
}

// TestVerifyTimestampAndTransparencyStates covers the ADR-005/ADR-006 gates:
// absence is reported but not fatal unless required, and a recorded absence is
// surfaced with its reason rather than silently ignored.
func TestVerifyTimestampAndTransparencyStates(t *testing.T) {
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)

	// No records: reported absent, not fatal.
	setVerifyFlags(t, "", false, "", false, "")
	res := verifyReceiptFile(path, ".", false, "")
	if !res.Valid {
		t.Fatalf("expected a clean signed receipt to be valid, got: %v", res.Errors)
	}
	if res.TimestampState != receipt.TimestampStateAbsent || res.TransparencyState != receipt.TransparencyStateAbsent {
		t.Errorf("states = %s/%s, want ABSENT/ABSENT", res.TimestampState, res.TransparencyState)
	}

	// Requiring them makes absence fatal.
	setVerifyFlags(t, "", true, "", false, "")
	if res := verifyReceiptFile(path, ".", false, ""); res.Valid {
		t.Error("--require-tsa should fail a receipt without a timestamp")
	}
	setVerifyFlags(t, "", false, "", true, "")
	if res := verifyReceiptFile(path, ".", false, ""); res.Valid {
		t.Error("--require-rekor should fail a receipt without a log entry")
	}

	// A recorded-but-absent record is surfaced with its reason.
	r.Timestamp = &receipt.TimestampAnchor{TSAURL: "https://tsa.example", Status: receipt.TimestampAbsent, Reason: "tsa unreachable"}
	r.Transparency = &receipt.TransparencyAnchor{LogURL: "https://rekor.example", Status: receipt.TransparencyAbsent, Reason: "log unreachable"}
	path2 := writeVerifyTestReceipt(t, &r)
	setVerifyFlags(t, "", false, "", false, "")
	res = verifyReceiptFile(path2, ".", false, "")
	if !res.Valid {
		t.Fatalf("absent records must not invalidate by default: %v", res.Errors)
	}
	if res.TimestampState != receipt.TimestampStateAbsent || res.TimestampReason != "tsa unreachable" {
		t.Errorf("timestamp = %s/%q, want ABSENT/%q", res.TimestampState, res.TimestampReason, "tsa unreachable")
	}
	if res.TransparencyState != receipt.TransparencyStateAbsent || res.TransparencyReason != "log unreachable" {
		t.Errorf("transparency = %s/%q, want ABSENT/%q", res.TransparencyState, res.TransparencyReason, "log unreachable")
	}
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

// TestVerifyRequireKeySourceCommand proves the ADR-008 trust-domain gate: a
// receipt signed by the agent's own key (the default file/env backend) is
// rejected when the verifier requires an external signer, while one signed by
// the command backend is accepted.
func TestVerifyRequireKeySourceCommand(t *testing.T) {
	// Default backend (KeySource=env/file): must fail the gate.
	envR := signedVerifyTestReceipt(t)
	envPath := writeVerifyTestReceipt(t, &envR)
	envFP := fingerprintOf(t, envR.PublicKey)
	setVerifyFlags(t, "", false, "", false, "command")
	res := verifyReceiptFile(envPath, t.TempDir(), false, envFP)
	if res.Valid {
		t.Fatalf("expected INVALID for env-sourced receipt under --require-keysource=command, got: %v", res.Errors)
	}
	if res.KeySourceState != receipt.KeySourceStateFailed {
		t.Errorf("key_source_state = %s, want FAILED", res.KeySourceState)
	}

	// Command backend (KeySource=command): must pass the gate. The gate is
	// enforced only with a pin, so the signer's key is pinned here.
	cmdR := commandSourcedReceiptSimulated(t)
	cmdPath := writeVerifyTestReceipt(t, &cmdR)
	cmdFP := fingerprintOf(t, cmdR.PublicKey)
	setVerifyFlags(t, "", false, "", false, "command")
	res = verifyReceiptFile(cmdPath, t.TempDir(), false, cmdFP)
	if !res.Valid {
		t.Fatalf("expected VALID for command-sourced receipt under --require-keysource=command, got: %v", res.Errors)
	}
	if res.KeySourceState != receipt.KeySourceStateOK {
		t.Errorf("key_source_state = %s, want OK", res.KeySourceState)
	}
}

// TestVerifyKeySourceStates checks the reported-but-not-gated and absent cases.
func TestVerifyKeySourceStates(t *testing.T) {
	// No gate: the source is reported but Unverified, and the receipt is valid.
	r := signedVerifyTestReceipt(t)
	path := writeVerifyTestReceipt(t, &r)
	setVerifyFlags(t, "", false, "", false, "")
	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if !res.Valid {
		t.Fatalf("expected VALID without gate, got: %v", res.Errors)
	}
	if res.KeySourceState != receipt.KeySourceStateUnverified {
		t.Errorf("key_source_state = %s, want UNVERIFIED", res.KeySourceState)
	}

	// A receipt recording no key_source (e.g. an unsigned/legacy one) is reported
	// Absent. It is invalid for other reasons (no signature), but the gate state
	// is Absent and is not what fails it.
	rNo := newVerifyTestReceipt() // unsigned, KeySource empty
	pathAbsent := writeVerifyTestReceipt(t, &rNo)
	setVerifyFlags(t, "", false, "", false, "")
	resAbsent := verifyReceiptFile(pathAbsent, t.TempDir(), false, "")
	if resAbsent.KeySourceState != receipt.KeySourceStateAbsent {
		t.Errorf("key_source_state = %s, want ABSENT", resAbsent.KeySourceState)
	}
}

// TestVerifyRequireKeySourceEndToEndCommand exercises the real ADR-008 command
// backend: it compiles the reference external signer, signs a receipt by
// delegating to it, then verifies with --require-keysource=command. This proves
// the whole out-of-process signing path works, not just a simulated field.
func TestVerifyRequireKeySourceEndToEndCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a helper binary")
	}
	bin := buildReferenceSigner(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "signer-key")
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(priv.Seed())), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("SELO_SIGNER", "command")
	t.Setenv("SELO_SIGNER_COMMAND", fmt.Sprintf("%s --key %s", bin, keyFile))
	t.Setenv("SELO_SIGNER_PUBKEY", base64.StdEncoding.EncodeToString(pub))

	r := newVerifyTestReceipt()
	if _, err := receipt.SignReceipt(&r); err != nil {
		t.Fatalf("SignReceipt via command backend: %v", err)
	}
	if r.KeySource != receipt.KeySourceCommand {
		t.Fatalf("KeySource = %q, want command", r.KeySource)
	}
	path := writeVerifyTestReceipt(t, &r)

	setVerifyFlags(t, "", false, "", false, "command")
	res := verifyReceiptFile(path, t.TempDir(), false, fingerprintOf(t, r.PublicKey))
	if !res.Valid {
		t.Fatalf("expected VALID command-sourced receipt, got: %v", res.Errors)
	}
	if res.KeySourceState != receipt.KeySourceStateOK {
		t.Errorf("key_source_state = %s, want OK", res.KeySourceState)
	}
}

// signedWithKeySource produces a receipt with a cryptographically valid
// signature that records the given key_source -- including the empty one.
// SignReceipt always stamps the source it resolved, so the only way to build
// the bypass shape (a valid signature over a receipt that omits the source) is
// to sign the canonical bytes directly. That is exactly what an attacker who
// holds the signing key would do, and it is the shape --require-keysource has
// to refuse.
func signedWithKeySource(t *testing.T, priv ed25519.PrivateKey, keySource string) receipt.ForgeReceipt {
	t.Helper()
	r := newVerifyTestReceipt()
	r.KeySource = keySource
	canonical, err := receipt.CanonicalJSON(&r)
	if err != nil {
		t.Fatalf("canonical json: %v", err)
	}
	sum := sha256.Sum256(canonical)
	r.ReceiptHash = hex.EncodeToString(sum[:])
	r.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical))
	r.PublicKey = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if ok, verr := receipt.VerifyReceipt(&r); verr != nil || !ok {
		t.Fatalf("fixture does not verify on its own: ok=%v err=%v", ok, verr)
	}
	return r
}

// TestVerifyRequireKeySourceFailClosed is the negative-path suite for the
// ADR-008 gate. A missing or emptied key_source must never count as
// compliance: the gate previously left KeySourceOK nil, and nil was read as
// "not evaluated, therefore pass", so clearing one field bypassed it.
func TestVerifyRequireKeySourceFailClosed(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// (a) Valid signature, empty key_source, gate set: must fail. The signer's
	// key is pinned so the only failure under test is the missing key_source.
	r := signedWithKeySource(t, priv, "")
	path := writeVerifyTestReceipt(t, &r)
	fp := fingerprintOf(t, r.PublicKey)
	setVerifyFlags(t, "", false, "", false, "command")
	res := verifyReceiptFile(path, t.TempDir(), false, fp)
	if res.Valid {
		t.Fatalf("expected INVALID: a missing key_source must not satisfy --require-keysource=command (state %s)", res.KeySourceState)
	}
	if res.KeySourceState != receipt.KeySourceStateAbsent {
		t.Errorf("key_source_state = %s, want ABSENT", res.KeySourceState)
	}
	if res.KeySourceOK == nil || *res.KeySourceOK {
		t.Errorf("key_source_ok = %v, want explicit false", res.KeySourceOK)
	}

	// (b) The same receipt without the gate is still valid: the fix must fail
	// closed on the gate without breaking ungated verification.
	setVerifyFlags(t, "", false, "", false, "")
	resUngated := verifyReceiptFile(path, t.TempDir(), false, "")
	if !resUngated.Valid {
		t.Fatalf("expected VALID without the gate, got: %v", resUngated.Errors)
	}

	// (c) key_source absent from the JSON entirely, not merely empty.
	raw, err := json.Marshal(&r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(obj, "key_source")
	stripped, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	strippedPath := filepath.Join(t.TempDir(), "no-field.json")
	if err := os.WriteFile(strippedPath, stripped, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setVerifyFlags(t, "", false, "", false, "command")
	resStripped := verifyReceiptFile(strippedPath, t.TempDir(), false, fp)
	if resStripped.Valid {
		t.Fatal("expected INVALID when key_source is absent from the JSON")
	}
	if resStripped.KeySourceState != receipt.KeySourceStateAbsent {
		t.Errorf("key_source_state = %s, want ABSENT", resStripped.KeySourceState)
	}
}

// TestVerifyRequireKeySourceNeedsPinning pins the gate's honest boundary.
// key_source is self-asserted: anyone holding a signing key can sign a receipt
// that claims "command" while having signed it in-process. The gate therefore
// only means something together with --pubkey, and is refused without one.
func TestVerifyRequireKeySourceNeedsPinning(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	// Signed in-process, but labelled as the external source.
	forged := signedWithKeySource(t, priv, receipt.KeySourceCommand)
	forgedPath := writeVerifyTestReceipt(t, &forged)

	// The gate alone cannot detect a falsely labelled source, so it is refused:
	// without a pinned key the result would look gated but prove nothing.
	setVerifyFlags(t, "", false, "", false, "command")
	resNoPin := verifyReceiptFile(forgedPath, t.TempDir(), false, "")
	if resNoPin.Valid {
		t.Fatalf("expected INVALID: --require-keysource without --pubkey must not pass, got %v", resNoPin.Errors)
	}
	if resNoPin.KeySourceState != receipt.KeySourceStateFailed {
		t.Errorf("key_source_state = %s, want FAILED", resNoPin.KeySourceState)
	}

	// Pinned to a different key, the forgery fails the pin: this is the check
	// that actually binds the signer.
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	otherFP := fingerprintOf(t, base64.StdEncoding.EncodeToString(otherPriv.Public().(ed25519.PublicKey)))
	setVerifyFlags(t, "", false, "", false, "command")
	resPinned := verifyReceiptFile(forgedPath, t.TempDir(), false, otherFP)
	if resPinned.Valid {
		t.Fatal("expected INVALID when a falsely labelled receipt is pinned to the real external signer")
	}
}

// TestVerifyRequireKeySourceNeedsPubkey covers the pair rule directly: the gate
// is a no-op without a pin, so the combination must be refused with a reason
// that names the missing flag.
func TestVerifyRequireKeySourceNeedsPubkey(t *testing.T) {
	cmdR := commandSourcedReceiptSimulated(t)
	path := writeVerifyTestReceipt(t, &cmdR)

	setVerifyFlags(t, "", false, "", false, "command")
	res := verifyReceiptFile(path, t.TempDir(), false, "")
	if res.Valid {
		t.Fatal("expected INVALID: --require-keysource with no --pubkey proves nothing")
	}
	if res.KeySourceState != receipt.KeySourceStateFailed {
		t.Errorf("key_source_state = %s, want FAILED", res.KeySourceState)
	}
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "--pubkey") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an error mentioning --pubkey, got %v", res.Errors)
	}
}

// TestVerifyCmdRequireKeySourceWithoutPubkeyIsUsageError checks the CLI refuses
// the pair before doing any verification, so a user cannot get a result that
// looks gated but is not.
func TestVerifyCmdRequireKeySourceWithoutPubkeyIsUsageError(t *testing.T) {
	cmdR := commandSourcedReceiptSimulated(t)
	path := writeVerifyTestReceipt(t, &cmdR)

	setVerifyFlags(t, "", false, "", false, "command")
	prevPub, prevJSON := verifyPubKey, verifyJSONOut
	verifyPubKey, verifyJSONOut = "", false
	t.Cleanup(func() { verifyPubKey, verifyJSONOut = prevPub, prevJSON })

	err := runVerifyCmd(nil, []string{path})
	if err == nil {
		t.Fatal("expected a usage error when --require-keysource is set without --pubkey")
	}
	if !strings.Contains(err.Error(), "--pubkey") {
		t.Errorf("error should mention --pubkey, got: %v", err)
	}
}

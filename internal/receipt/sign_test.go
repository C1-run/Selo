package receipt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sampleReceipt() *ForgeReceipt {
	return &ForgeReceipt{
		ReceiptID: "c1f-test",
		TaskID:    "run-test",
		Verdict:   "NOOP_WITH_RECEIPT",
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	seedB64, _, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	t.Setenv("SELO_SIGNING_KEY", seedB64)
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	r := sampleReceipt()
	if _, err := SignReceipt(r); err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	if r.Signature == "" || r.PublicKey == "" || r.ReceiptHash == "" {
		t.Fatal("SignReceipt did not populate signature/publicKey/receiptHash")
	}
	ok, err := VerifyReceipt(r)
	if err != nil {
		t.Fatalf("VerifyReceipt: %v", err)
	}
	if !ok {
		t.Fatal("expected a valid signature to verify")
	}
}

func TestVerifyDetectsTamper(t *testing.T) {
	seedB64, _, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	t.Setenv("SELO_SIGNING_KEY", seedB64)
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	r := sampleReceipt()
	if _, err := SignReceipt(r); err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	// Tamper with a field that IS covered by the canonical signature.
	r.Verdict = "SUCCESS_WITH_RECEIPT"
	ok, err := VerifyReceipt(r)
	if err != nil {
		t.Fatalf("VerifyReceipt: %v", err)
	}
	if ok {
		t.Fatal("tampered receipt must NOT verify")
	}
}

func TestCanonicalJSONStableAcrossSignature(t *testing.T) {
	a := sampleReceipt()
	b := sampleReceipt()
	ca, _ := CanonicalJSON(a)
	cb, _ := CanonicalJSON(b)
	if string(ca) != string(cb) {
		t.Fatal("canonical JSON must be independent of signature/anchor fields")
	}
}

func TestLoadSigningKeyFromEnv(t *testing.T) {
	seedB64, pubB64, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	os.Setenv("SELO_SIGNING_KEY", seedB64)
	defer os.Unsetenv("SELO_SIGNING_KEY")

	priv, gotPub, err := LoadSigningKey()
	if err != nil {
		t.Fatalf("LoadSigningKey: %v", err)
	}
	if priv == nil {
		t.Fatal("expected a non-nil private key")
	}
	if gotPub != pubB64 {
		t.Fatalf("public key mismatch: got %s want %s", gotPub, pubB64)
	}
}

func TestVerifyReceiptData(t *testing.T) {
	seedB64, _, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	t.Setenv("SELO_SIGNING_KEY", seedB64)
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	r := sampleReceipt()
	sig, err := SignReceipt(r)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	canonical, _ := CanonicalJSON(r)
	ok, err := VerifyReceiptData(canonical, sig, r.PublicKey)
	if err != nil {
		t.Fatalf("VerifyReceiptData: %v", err)
	}
	if !ok {
		t.Fatal("expected VerifyReceiptData to succeed")
	}
}

func TestSignRecordsPersistentKeyMode(t *testing.T) {
	seedB64, _, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	t.Setenv("SELO_SIGNING_KEY", seedB64)
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	r := sampleReceipt()
	if _, err := SignReceipt(r); err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	if r.KeyMode != KeyModePersistent {
		t.Fatalf("expected key_mode %q, got %q", KeyModePersistent, r.KeyMode)
	}
	// Key mode must be covered by the signature: tampering with it breaks verify.
	ok, err := VerifyReceipt(r)
	if err != nil {
		t.Fatalf("VerifyReceipt: %v", err)
	}
	if !ok {
		t.Fatal("expected valid signature with persistent key_mode")
	}
	r.KeyMode = KeyModeEphemeral
	if ok, _ := VerifyReceipt(r); ok {
		t.Fatal("changing key_mode must invalidate the signature")
	}
}

func TestSignFailClosedWithoutKey(t *testing.T) {
	// Isolate HOME so a developer's real ~/.selo/signing-key can't leak in.
	t.Setenv("HOME", t.TempDir())
	os.Unsetenv("SELO_SIGNING_KEY")
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	r := sampleReceipt()
	_, err := SignReceipt(r)
	if err == nil {
		t.Fatal("expected SignReceipt to fail when no key is configured")
	}
	if !errors.Is(err, ErrNoSigningKey) {
		t.Fatalf("expected ErrNoSigningKey, got %v", err)
	}
	if r.KeyMode != "" {
		t.Fatalf("expected key_mode unset on failure, got %q", r.KeyMode)
	}
}

// writeSigningKey writes a valid seed to $HOME/.selo/signing-key under a fresh
// HOME and returns the matching public key. Used to exercise auto-loading.
func writeSigningKey(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv("SELO_SIGNING_KEY")
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	seedB64, pubB64, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	dir := filepath.Join(home, ".selo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "signing-key"), []byte(seedB64+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return pubB64
}

func TestLoadSigningKeyFromFileFallback(t *testing.T) {
	wantPub := writeSigningKey(t)

	priv, gotPub, mode, err := LoadSigningKeyWithMode()
	if err != nil {
		t.Fatalf("LoadSigningKeyWithMode: %v", err)
	}
	if priv == nil {
		t.Fatal("expected a non-nil private key from the fallback file")
	}
	if gotPub != wantPub {
		t.Fatalf("public key mismatch: got %s want %s", gotPub, wantPub)
	}
	if mode != KeyModePersistent {
		t.Fatalf("expected key_mode %q, got %q", KeyModePersistent, mode)
	}
}

func TestSignUsesKeyFileWithoutEnv(t *testing.T) {
	writeSigningKey(t)

	r := sampleReceipt()
	if _, err := SignReceipt(r); err != nil {
		t.Fatalf("SignReceipt should auto-load the key file, got: %v", err)
	}
	if r.KeyMode != KeyModePersistent {
		t.Fatalf("expected key_mode %q, got %q", KeyModePersistent, r.KeyMode)
	}
	if ok, err := VerifyReceipt(r); err != nil || !ok {
		t.Fatalf("auto-loaded receipt must verify: ok=%v err=%v", ok, err)
	}
}

func TestMalformedKeyFileFailsLoud(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv("SELO_SIGNING_KEY")
	os.Unsetenv("SELO_ALLOW_EPHEMERAL_KEY")

	dir := filepath.Join(home, ".selo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "signing-key"), []byte("not-valid-base64!!!"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// A present-but-broken key must error, not silently fall back to ephemeral.
	if _, _, _, err := LoadSigningKeyWithMode(); err == nil {
		t.Fatal("expected an error for a malformed key file")
	}
}

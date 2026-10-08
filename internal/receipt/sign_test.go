package receipt

import (
	"errors"
	"os"
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

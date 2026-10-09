package receipt

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeSecurity writes a shim named `security` onto PATH that emulates the
// macOS keychain CLI against a file, so the keychain backend can be tested
// without touching (or prompting) the real login keychain.
func fakeSecurity(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "secret")
	script := `#!/bin/sh
cmd="$1"; shift
case "$cmd" in
  add-generic-password)
    prev=""
    for a in "$@"; do
      if [ "$prev" = "-w" ]; then printf '%s' "$a" > "$STORE"; fi
      prev="$a"
    done
    ;;
  find-generic-password)
    [ -f "$STORE" ] || exit 1
    cat "$STORE"
    ;;
  delete-generic-password)
    rm -f "$STORE"
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "security"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORE", store)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestKeychainRoundTripWithFakeCLI(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the fake `security` shim exercises the macOS keychain code path")
	}
	fakeSecurity(t)

	if _, err := KeychainGet("svc"); err == nil {
		t.Fatal("expected an error for a missing keychain entry")
	}
	if err := KeychainSet("svc", "seed-value"); err != nil {
		t.Fatalf("KeychainSet: %v", err)
	}
	got, err := KeychainGet("svc")
	if err != nil {
		t.Fatalf("KeychainGet: %v", err)
	}
	if got != "seed-value" {
		t.Fatalf("got %q, want seed-value", got)
	}
	if err := KeychainDelete("svc"); err != nil {
		t.Fatalf("KeychainDelete: %v", err)
	}
	if _, err := KeychainGet("svc"); err == nil {
		t.Fatal("expected an error after delete")
	}
}

// TestSignReceiptViaKeychainBackend is the end-to-end guard for ADR-008 Phase 2:
// with SELO_SIGNER=keychain, a receipt signs and verifies, and records the
// keychain key source.
func TestSignReceiptViaKeychainBackend(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the fake `security` shim exercises the macOS keychain code path")
	}
	seedB64, pubB64, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	store := filepath.Join(dir, "secret")
	if err := os.WriteFile(store, []byte(seedB64), 0644); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
cmd="$1"; shift
case "$cmd" in
  find-generic-password) cat "$STORE" ;;
  add-generic-password) ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "security"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORE", store)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SELO_SIGNER", "keychain")
	t.Setenv("SELO_SIGNING_KEY", "")

	s, err := ResolveSigner()
	if err != nil {
		t.Fatalf("ResolveSigner: %v", err)
	}
	if s.KeySource() != KeySourceKeychain {
		t.Fatalf("source = %s, want keychain", s.KeySource())
	}
	if s.PublicKeyB64() != pubB64 {
		t.Fatal("keychain public key does not match the generated key")
	}

	r := &ForgeReceipt{ReceiptID: "kc-1", TaskID: "t", Verdict: VerdictSuccess}
	if _, err := SignReceipt(r); err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	if r.KeySource != KeySourceKeychain {
		t.Fatalf("receipt key_source = %q, want keychain", r.KeySource)
	}
	if ok, err := VerifyReceipt(r); err != nil || !ok {
		t.Fatalf("VerifyReceipt: ok=%v err=%v", ok, err)
	}
}

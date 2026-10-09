package receipt

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// TestCommandSignerUsesExternalSignature pins the external-signer contract: Selo
// writes the message to stdin and reads a base64 Ed25519 signature from stdout.
// The stub returns a real signature so the result is verified, not just parsed.
func TestCommandSignerUsesExternalSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("bytes to be signed")
	sigB64 := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))

	dir := t.TempDir()
	script := filepath.Join(dir, "sign.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' "+sigB64+"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	s := &commandSigner{cmd: "sh", args: []string{script}, pubB64: base64.StdEncoding.EncodeToString(pub)}
	got, err := s.Sign(msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ed25519.Verify(pub, msg, got) {
		t.Fatal("command signer produced a signature that does not verify")
	}
	if s.KeySource() != KeySourceCommand || s.KeyMode() != KeyModePersistent {
		t.Fatalf("unexpected mode/source: %s/%s", s.KeyMode(), s.KeySource())
	}
}

// TestCommandSignerRejectsBadOutput guards against a signer that prints
// something that is not a base64 signature.
func TestCommandSignerRejectsBadOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat >/dev/null\nprintf 'not-base64!!'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	s := &commandSigner{cmd: "sh", args: []string{script}}
	if _, err := s.Sign([]byte("x")); err == nil {
		t.Fatal("expected an error for non-base64 signer output")
	}
}

func TestResolveSignerCommand(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	t.Setenv("SELO_SIGNER", "command")
	t.Setenv("SELO_SIGNER_COMMAND", "sh -c true")
	t.Setenv("SELO_SIGNER_PUBKEY", pubB64)

	s, err := ResolveSigner()
	if err != nil {
		t.Fatalf("ResolveSigner: %v", err)
	}
	if s.KeySource() != KeySourceCommand {
		t.Fatalf("source = %s, want command", s.KeySource())
	}
	if s.PublicKeyB64() != pubB64 {
		t.Fatal("resolved public key does not match SELO_SIGNER_PUBKEY")
	}
}

func TestResolveSignerCommandNeedsEnv(t *testing.T) {
	t.Setenv("SELO_SIGNER", "command")
	t.Setenv("SELO_SIGNER_COMMAND", "")
	t.Setenv("SELO_SIGNER_PUBKEY", "")
	if _, err := ResolveSigner(); err == nil {
		t.Fatal("expected an error when the command signer env is incomplete")
	}
}

func TestResolveSignerUnknownBackend(t *testing.T) {
	t.Setenv("SELO_SIGNER", "hsm")
	if _, err := ResolveSigner(); err == nil {
		t.Fatal("expected an error for an unknown SELO_SIGNER")
	}
}

func TestResolveSignerDefaultIsFileSource(t *testing.T) {
	seedB64, _, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELO_SIGNER", "")
	t.Setenv("SELO_SIGNING_KEY", seedB64)

	s, err := ResolveSigner()
	if err != nil {
		t.Fatalf("ResolveSigner: %v", err)
	}
	if s.KeySource() != KeySourceEnv || s.KeyMode() != KeyModePersistent {
		t.Fatalf("mode/source = %s/%s, want persistent/env", s.KeyMode(), s.KeySource())
	}
}

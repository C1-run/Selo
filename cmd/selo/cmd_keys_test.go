package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/desmondkam/selo/internal/receipt"
)

func TestKeysGenerateAndPub(t *testing.T) {
	prevOut, prevForce, prevIn := keysOut, keysForce, keysPubIn
	defer func() { keysOut, keysForce, keysPubIn = prevOut, prevForce, prevIn }()

	out := filepath.Join(t.TempDir(), "signing-key")
	keysOut, keysForce, keysPubIn = out, false, out

	if err := runKeysGenerate(nil, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("key file mode = %v, want 0600", info.Mode().Perm())
	}
	seed, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Regenerating without --force must refuse.
	if err := runKeysGenerate(nil, nil); err == nil {
		t.Error("second generate without --force should be refused")
	}

	// pub must print the public key matching the stored seed.
	t.Setenv("SELO_SIGNING_KEY", strings.TrimSpace(string(seed)))
	_, wantPub, err := receipt.LoadSigningKey()
	if err != nil {
		t.Fatalf("load signing key: %v", err)
	}
	got := captureStdout(t, func() { runKeysPub(nil, nil) })
	if strings.TrimSpace(got) != wantPub {
		t.Errorf("pub = %q, want %q", strings.TrimSpace(got), wantPub)
	}
}

// Command selo-signer is a REFERENCE external signer for Selo's ADR-008
// `command` backend. Selo invokes it with the message on stdin and reads a
// base64 Ed25519 signature from stdout:
//
//	selo-signer --key <path-to-private-key>
//
// It is meant to run in a SEPARATE trust domain from the audited agent. The
// private key must live where the agent process cannot read it — a different
// Unix user, a container, an HSM/TPM, or a CI signing job. Selo itself never
// sees the private key: it only knows the matching public key, which you pass
// to Selo via SELO_SIGNER_PUBKEY. That separation is what makes a receipt
// non-repudiable rather than merely tamper-evident.
//
// The key file (<key>) holds, in order of preference: a base64-encoded 32-byte
// Ed25519 seed, a base64-encoded 64-byte private key, or a PEM PKCS#8 private
// key.
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	keyPath := ""
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--key":
			if i+1 < len(os.Args) {
				keyPath = os.Args[i+1]
				i++
			}
		case "--key=" + os.Args[i][len("--key="):]:
			keyPath = os.Args[i][len("--key="):]
		}
	}
	if keyPath == "" {
		keyPath = os.Getenv("SELO_SIGNER_KEY_FILE")
	}
	if keyPath == "" {
		fmt.Fprintln(os.Stderr, "selo-signer: --key <file> or SELO_SIGNER_KEY_FILE required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "selo-signer: read key: %v\n", err)
		os.Exit(2)
	}
	priv, err := parseEd25519Private(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "selo-signer: %v\n", err)
		os.Exit(2)
	}

	msg, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "selo-signer: read stdin: %v\n", err)
		os.Exit(2)
	}

	sig := ed25519.Sign(priv, msg)
	fmt.Print(base64.StdEncoding.EncodeToString(sig))
}

// parseEd25519Private accepts a base64 32-byte seed, a base64 64-byte private
// key, or a PEM PKCS#8 private key.
func parseEd25519Private(b []byte) (ed25519.PrivateKey, error) {
	s := strings.TrimSpace(string(b))
	if dec, err := base64.StdEncoding.DecodeString(s); err == nil {
		switch len(dec) {
		case ed25519.SeedSize:
			return ed25519.NewKeyFromSeed(dec), nil
		case ed25519.PrivateKeySize:
			return ed25519.PrivateKey(dec), nil
		}
	}
	if blk, _ := pem.Decode(b); blk != nil {
		if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
			if pk, ok := k.(ed25519.PrivateKey); ok {
				return pk, nil
			}
			return nil, fmt.Errorf("PEM key is %T, want ed25519", k)
		}
	}
	return nil, fmt.Errorf("unsupported private key format (want base64 ed25519 seed/key or PEM PKCS#8)")
}

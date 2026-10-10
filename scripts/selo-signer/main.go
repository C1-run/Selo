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
// to Selo via SELO_SIGNER_PUBKEY. That separation is what lets a verifier
// attribute a receipt to the operator rather than merely call it tamper-evident.
//
// WARNING: this program is a signing oracle for whatever can invoke it. Run it
// where the audited agent cannot reach it. If the agent can run it, it can
// obtain a signature and the attribution property is void — the payload guard
// below only narrows what it will sign, it is not a substitute for isolation.
//
// The key file (<key>) holds, in order of preference: a base64-encoded 32-byte
// Ed25519 seed, a base64-encoded 64-byte private key, or a PEM PKCS#8 private
// key.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	keyPath := ""
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--key" {
			if i+1 < len(os.Args) {
				keyPath = os.Args[i+1]
				i++
			}
			continue
		}
		if v, ok := strings.CutPrefix(arg, "--key="); ok {
			keyPath = v
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
	if !looksLikeSeloPayload(msg) {
		fmt.Fprintln(os.Stderr, "selo-signer: refusing to sign: stdin is not a Selo receipt or a DSSE PAE")
		os.Exit(3)
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

// looksLikeSeloPayload reports whether b is something Selo is expected to sign:
// a receipt's canonical JSON (an object carrying receipt_id and task_id) or a
// DSSE PAE ("DSSEv1 ..."). Anything else is refused, so a caller that reaches
// the signer cannot use it as a general-purpose signing oracle for arbitrary
// bytes. It does not make the signer safe to expose: it remains an oracle for
// these two shapes, so keeping it unreachable by the agent is still what
// matters.
func looksLikeSeloPayload(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	if bytes.HasPrefix(trimmed, []byte("DSSEv1 ")) {
		return true
	}
	var probe struct {
		ReceiptID string `json:"receipt_id"`
		TaskID    string `json:"task_id"`
	}
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return false
	}
	return probe.ReceiptID != "" && probe.TaskID != ""
}

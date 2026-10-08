package receipt

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	ephemeralPriv ed25519.PrivateKey
	ephemeralPub  string
	ephemeralOnce sync.Once
)

// Key modes recorded on every receipt.
const (
	KeyModePersistent = "persistent"
	KeyModeEphemeral  = "ephemeral"
)

// ErrNoSigningKey is returned when no persistent key is configured and
// ephemeral keys are not explicitly allowed. Signing fails closed: a receipt
// signed by a key nobody can attribute is worth nothing as evidence, so Selo
// refuses to produce one rather than quietly producing a weak one.
var ErrNoSigningKey = errors.New("no signing key configured")

// EphemeralKeyAllowed reports whether the caller explicitly opted in to a
// per-process key, either with SELO_ALLOW_EPHEMERAL_KEY=1 or (from `selo run`)
// with --dev.
func EphemeralKeyAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SELO_ALLOW_EPHEMERAL_KEY"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func missingKeyError() error {
	return fmt.Errorf("%w: set SELO_SIGNING_KEY (run `selo keys generate`) so receipts are "+
		"attributable across runs, or set SELO_ALLOW_EPHEMERAL_KEY=1 / pass --dev to accept a "+
		"per-process key that nobody can verify you signed", ErrNoSigningKey)
}

// LoadSigningKey loads Ed25519 private key from SELO_SIGNING_KEY env var and
// reports the key mode. See LoadSigningKeyWithMode for the full contract.
func LoadSigningKey() (ed25519.PrivateKey, string, error) {
	priv, pub, _, err := LoadSigningKeyWithMode()
	return priv, pub, err
}

// LoadSigningKeyWithMode loads the Ed25519 private key and reports how
// attributable it is.
//
// Expects SELO_SIGNING_KEY to hold a base64 32-byte seed or 64-byte private
// key. If it is unset, signing fails closed with ErrNoSigningKey unless
// SELO_ALLOW_EPHEMERAL_KEY is set, in which case a per-process key is
// generated, cached, and reported as KeyModeEphemeral.
func LoadSigningKeyWithMode() (ed25519.PrivateKey, string, string, error) {
	b64 := os.Getenv("SELO_SIGNING_KEY")
	if b64 == "" {
		if !EphemeralKeyAllowed() {
			return nil, "", "", missingKeyError()
		}
		// Dev mode: ephemeral key (cached)
		var loadErr error
		ephemeralOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "[selo] ⚠️  SELO_SIGNING_KEY not set — generating ephemeral key (dev only, not for production)")
			var priv ed25519.PrivateKey
			_, priv, loadErr = ed25519.GenerateKey(rand.Reader)
			if loadErr == nil {
				ephemeralPriv = priv
				ephemeralPub = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
			}
		})
		if loadErr != nil {
			return nil, "", "", fmt.Errorf("generate ephemeral key: %w", loadErr)
		}
		return ephemeralPriv, ephemeralPub, KeyModeEphemeral, nil
	}
	mode := KeyModePersistent
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, "", "", fmt.Errorf("SELO_SIGNING_KEY base64 decode: %w", err)
	}
	var priv ed25519.PrivateKey
	switch len(raw) {
	case 32:
		priv = ed25519.NewKeyFromSeed(raw)
	case 64:
		priv = ed25519.PrivateKey(raw)
	default:
		return nil, "", "", fmt.Errorf("SELO_SIGNING_KEY must be 32 or 64 bytes (got %d)", len(raw))
	}
	pubB64 := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	return priv, pubB64, mode, nil
}

// PublicKeyFingerprint returns a stable hex fingerprint of a base64-encoded
// ed25519 public key, used to pin a signer without handling raw key material.
func PublicKeyFingerprint(pubB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return "", fmt.Errorf("public key base64 decode: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("public key must be %d bytes (got %d)", ed25519.PublicKeySize, len(raw))
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// GenerateKeyPair creates a new Ed25519 keypair and returns base64-encoded seed and pubkey.
func GenerateKeyPair() (seedB64, pubB64 string, priv ed25519.PrivateKey, err error) {
	_, priv, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", nil, err
	}
	seed := priv.Seed()
	seedB64 = base64.StdEncoding.EncodeToString(seed)
	pubB64 = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	return seedB64, pubB64, priv, nil
}

// CanonicalJSON returns the canonical JSON of receipt excluding signature/anchor fields.
// Used for signing and verification. Must be deterministic.
func CanonicalJSON(r *ForgeReceipt) ([]byte, error) {
	// Copy without signature/anchor fields to avoid circular signing
	cp := *r
	cp.Signature = ""
	cp.PublicKey = ""
	cp.ReceiptHash = ""
	cp.AnchorCommit = ""
	cp.AnchorBranch = ""
	cp.AnchoredAt = nil
	return json.Marshal(cp)
}

// SignReceipt signs the receipt's canonical JSON and sets Signature/PublicKey/ReceiptHash.
// Returns signature base64.
func SignReceipt(r *ForgeReceipt) (string, error) {
	priv, pubB64, mode, err := LoadSigningKeyWithMode()
	if err != nil {
		return "", err
	}
	// Record the key mode on the receipt *before* canonicalizing so it is
	// covered by the signature. A forged receipt that lies about its mode
	// (e.g. claims "persistent" while signed by an ephemeral key) will fail
	// signature verification because the canonical bytes differ.
	r.KeyMode = mode
	canonical, err := CanonicalJSON(r)
	if err != nil {
		return "", fmt.Errorf("canonical json: %w", err)
	}
	hash := sha256.Sum256(canonical)
	r.ReceiptHash = hex.EncodeToString(hash[:])
	sig := ed25519.Sign(priv, canonical)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	r.Signature = sigB64
	r.PublicKey = pubB64
	return sigB64, nil
}

// VerifyReceipt verifies the receipt's signature against its canonical JSON.
func VerifyReceipt(r *ForgeReceipt) (bool, error) {
	if r.Signature == "" {
		return false, fmt.Errorf("receipt has no signature")
	}
	if r.PublicKey == "" {
		return false, fmt.Errorf("receipt has no public key")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil {
		return false, fmt.Errorf("signature base64 decode: %w", err)
	}
	pub, err := base64.StdEncoding.DecodeString(r.PublicKey)
	if err != nil {
		return false, fmt.Errorf("public key base64 decode: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key size %d", len(pub))
	}
	canonical, err := CanonicalJSON(r)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(ed25519.PublicKey(pub), canonical, sig), nil
}

// VerifyReceiptData verifies raw receipt JSON bytes against signature and pubkey.
func VerifyReceiptData(canonicalJSON []byte, sigB64, pubB64 string) (bool, error) {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false, err
	}
	pub, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(ed25519.PublicKey(pub), canonicalJSON, sig), nil
}

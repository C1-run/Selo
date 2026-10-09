package receipt

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Signer produces an Ed25519 signature over the bytes Selo signs — the
// receipt's canonical JSON, or the DSSE PAE for an attestation. Abstracting it
// lets the signing key live somewhere other than a file the audited agent can
// read (ADR-008).
type Signer interface {
	// PublicKeyB64 is the base64 ed25519 public key the receipt embeds.
	PublicKeyB64() string
	// KeyMode is persistent (attributable across runs) or ephemeral.
	KeyMode() string
	// KeySource is where the key came from: env | file | keychain | command |
	// ephemeral. It is recorded on the receipt, signed.
	KeySource() string
	// Sign returns a raw 64-byte Ed25519 signature over msg.
	Sign(msg []byte) ([]byte, error)
}

// localSigner signs in-process with a key Selo loaded from env, a file, or the
// keychain.
type localSigner struct {
	priv   ed25519.PrivateKey
	pubB64 string
	mode   string
	source string
}

func (s *localSigner) PublicKeyB64() string { return s.pubB64 }
func (s *localSigner) KeyMode() string      { return s.mode }
func (s *localSigner) KeySource() string    { return s.source }

func (s *localSigner) Sign(msg []byte) ([]byte, error) {
	return ed25519.Sign(s.priv, msg), nil
}

// commandSigner delegates signing to an external program, so the private key
// never enters Selo or the filesystem the agent can read. This is the escape
// hatch for a KMS, an HSM, ssh-agent, a separate signer user, or a CI signing
// job.
//
// Protocol: Selo writes the message to the command's stdin and reads a base64
// Ed25519 signature from its stdout. The matching public key is supplied out of
// band via SELO_SIGNER_PUBKEY, because it must be known before signing — it is
// embedded in the receipt.
type commandSigner struct {
	cmd    string
	args   []string
	pubB64 string
}

func (s *commandSigner) PublicKeyB64() string { return s.pubB64 }
func (s *commandSigner) KeyMode() string      { return KeyModePersistent }
func (s *commandSigner) KeySource() string    { return KeySourceCommand }

func (s *commandSigner) Sign(msg []byte) ([]byte, error) {
	c := exec.Command(s.cmd, s.args...)
	c.Stdin = bytes.NewReader(msg)
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("signer command %q failed: %v: %s", s.cmd, err, strings.TrimSpace(errb.String()))
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(out.String()))
	if err != nil {
		return nil, fmt.Errorf("signer command output is not base64: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signer command returned a %d-byte signature, want %d", len(sig), ed25519.SignatureSize)
	}
	return sig, nil
}

// ResolveSigner picks the signing backend from SELO_SIGNER:
//
//	file (default) → SELO_SIGNING_KEY, then ~/.selo/signing-key, then an
//	                 ephemeral key if explicitly allowed, else fail closed.
//	keychain       → the OS keychain entry (`selo keys store --keychain`).
//	command        → an external program (SELO_SIGNER_COMMAND).
//
// See ADR-008: only `command` (with a non-extractable key) puts the signer
// outside the agent's trust domain; `file` and `keychain` keep the key readable
// by the same user.
func ResolveSigner() (Signer, error) {
	switch kind := strings.ToLower(strings.TrimSpace(os.Getenv("SELO_SIGNER"))); kind {
	case "command":
		cmdline := strings.TrimSpace(os.Getenv("SELO_SIGNER_COMMAND"))
		pub := strings.TrimSpace(os.Getenv("SELO_SIGNER_PUBKEY"))
		if cmdline == "" || pub == "" {
			return nil, fmt.Errorf("SELO_SIGNER=command needs both SELO_SIGNER_COMMAND and SELO_SIGNER_PUBKEY")
		}
		if _, err := PublicKeyFingerprint(pub); err != nil {
			return nil, fmt.Errorf("SELO_SIGNER_PUBKEY: %w", err)
		}
		fields := strings.Fields(cmdline)
		return &commandSigner{cmd: fields[0], args: fields[1:], pubB64: pub}, nil

	case "keychain":
		secret, err := KeychainGet(DefaultKeychainService)
		if err != nil {
			return nil, fmt.Errorf("SELO_SIGNER=keychain: %w (store one with `selo keys store --keychain`)", err)
		}
		priv, pub, err := parseKeyMaterial(secret, "keychain")
		if err != nil {
			return nil, err
		}
		return &localSigner{priv: priv, pubB64: pub, mode: KeyModePersistent, source: KeySourceKeychain}, nil

	case "", "file":
		priv, pub, mode, source, err := loadLocalKey()
		if err != nil {
			return nil, err
		}
		return &localSigner{priv: priv, pubB64: pub, mode: mode, source: source}, nil

	default:
		return nil, fmt.Errorf("unknown SELO_SIGNER %q (use file, keychain, or command)", kind)
	}
}

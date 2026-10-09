package receipt

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

// in-toto / DSSE constants (ADR-001).
const (
	// StatementType is the in-toto Statement v1 media type.
	StatementType = "https://in-toto.io/Statement/v1"
	// PredicateType identifies Selo's receipt predicate. It is versioned and
	// intended to be dereferenceable.
	PredicateType = "https://selo.c1.run/attestation/v1"
	// DSSEPayloadType is the DSSE payloadType for an in-toto statement.
	DSSEPayloadType = "application/vnd.in-toto+json"
)

// Subject binds an attestation to the artifact it is about. For Selo that is
// the audited change, identified by the sha256 of its diff (diff_hash).
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Statement is an in-toto Statement v1.
type Statement struct {
	Type          string          `json:"_type"`
	Subject       []Subject       `json:"subject"`
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

// Signature is one DSSE signature.
type Signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

// Envelope is a DSSE envelope (secure-systems-lab/dsse).
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []Signature `json:"signatures"`
}

// PAE is the DSSE pre-authentication encoding: the exact bytes that are signed.
//
//	PAE(type, payload) = "DSSEv1" SP len(type) SP type SP len(payload) SP payload
//
// where len is the decimal byte length and SP is a single space. Signing this,
// rather than a re-serialization of a struct, is what lets a verifier in any
// language check a receipt without reimplementing Go's JSON encoding.
func PAE(payloadType string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString("DSSEv1")
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(len(payloadType)))
	b.WriteByte(' ')
	b.WriteString(payloadType)
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(len(payload)))
	b.WriteByte(' ')
	b.Write(payload)
	return b.Bytes()
}

// BuildStatement wraps a receipt as an in-toto Statement v1 whose subject is
// the audited change (sha256 of the diff, falling back to the receipt content
// hash when no diff was recorded).
func BuildStatement(r *ForgeReceipt) ([]byte, error) {
	digest := r.DiffHash
	if digest == "" {
		digest = r.ReceiptHash
	}
	name := r.TaskID
	if name == "" {
		name = r.ReceiptID
	}
	pred, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshal predicate: %w", err)
	}
	stmt := Statement{
		Type:          StatementType,
		Subject:       []Subject{{Name: "selo://" + name, Digest: map[string]string{"sha256": digest}}},
		PredicateType: PredicateType,
		Predicate:     pred,
	}
	return json.Marshal(stmt)
}

// SignStatement wraps payload in a DSSE envelope and signs the PAE with priv.
func SignStatement(payload []byte, payloadType string, priv ed25519.PrivateKey, keyID string) (*Envelope, error) {
	sig := ed25519.Sign(priv, PAE(payloadType, payload))
	return &Envelope{
		PayloadType: payloadType,
		Payload:     base64.StdEncoding.EncodeToString(payload),
		Signatures:  []Signature{{KeyID: keyID, Sig: base64.StdEncoding.EncodeToString(sig)}},
	}, nil
}

// VerifyStatement checks a DSSE envelope's signature against pubB64 and returns
// the decoded payload bytes. It reports (payload, true, nil) on a valid
// signature, (payload, false, nil) on a well-formed but invalid one, and an
// error when the envelope itself is malformed.
func VerifyStatement(env *Envelope, pubB64 string) ([]byte, bool, error) {
	if env.PayloadType == "" || env.Payload == "" || len(env.Signatures) == 0 {
		return nil, false, fmt.Errorf("malformed DSSE envelope")
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, false, fmt.Errorf("payload base64 decode: %w", err)
	}
	pub, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return nil, false, fmt.Errorf("public key base64 decode: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, false, fmt.Errorf("public key must be %d bytes (got %d)", ed25519.PublicKeySize, len(pub))
	}
	pae := PAE(env.PayloadType, payload)
	for _, s := range env.Signatures {
		sig, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(pub), pae, sig) {
			return payload, true, nil
		}
	}
	return payload, false, nil
}

// ExportInToto builds and signs an in-toto attestation for a receipt, returning
// the DSSE envelope. The signing key is resolved exactly like receipt signing
// (SELO_SIGNING_KEY, then ~/.selo/signing-key; fail-closed with no key).
func ExportInToto(r *ForgeReceipt) (*Envelope, error) {
	stmt, err := BuildStatement(r)
	if err != nil {
		return nil, err
	}
	priv, pubB64, _, err := LoadSigningKeyWithMode()
	if err != nil {
		return nil, err
	}
	keyID, err := PublicKeyFingerprint(pubB64)
	if err != nil {
		return nil, err
	}
	return SignStatement(stmt, DSSEPayloadType, priv, keyID)
}

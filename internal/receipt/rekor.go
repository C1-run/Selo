package receipt

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Transparency status values recorded in TransparencyAnchor.Status.
const (
	TransparencyLogged = "logged"
	TransparencyAbsent = "absent"
)

// Transparency state values reported by verification.
const (
	TransparencyStateOK         = "OK"
	TransparencyStateUnverified = "UNVERIFIED"
	TransparencyStateFailed     = "FAILED"
	TransparencyStateAbsent     = "ABSENT"
	TransparencyStateSkipped    = "SKIPPED"
)

// maxRekorResponseBytes bounds how much data a Rekor response can make Selo read.
const maxRekorResponseBytes = 4 << 20 // 4 MiB

// rekorHTTPTimeout bounds the Rekor round-trip. A variable so tests can shorten it.
var rekorHTTPTimeout = 30 * time.Second

// TransparencyAnchor is a Sigstore Rekor transparency-log record for a receipt
// (ADR-006).
//
// It is produced *after* signing — the log accepts an already-signed entry — so
// like the anchor and timestamp fields it is excluded from CanonicalJSON.
//
// Privacy: Selo logs a DSSE entry whose payload is the receipt hash. Rekor
// canonicalizes a DSSE entry to the envelope and payload *hashes*, so no task
// name, file path, or receipt content is ever published.
type TransparencyAnchor struct {
	LogURL         string               `json:"log_url"`
	UUID           string               `json:"uuid,omitempty"`
	LogID          string               `json:"log_id,omitempty"`
	LogIndex       int64                `json:"log_index"`
	IntegratedTime int64                `json:"integrated_time,omitempty"` // unix seconds, issued by the log
	PayloadHash    string               `json:"payload_hash"`              // hex sha256 of the logged payload; this is what the entry commits to
	EnvelopeHash   string               `json:"envelope_hash,omitempty"`   // hex sha256 of the DSSE envelope
	Body           string               `json:"body,omitempty"`            // base64 canonicalized log entry
	SET            string               `json:"signed_entry_timestamp,omitempty"`
	InclusionProof *RekorInclusionProof `json:"inclusion_proof,omitempty"`
	Status         string               `json:"status"` // logged | absent
	Reason         string               `json:"reason,omitempty"`
}

// RekorInclusionProof is an RFC 6962 audit path from a leaf to a signed tree
// head, as returned by the log.
type RekorInclusionProof struct {
	Checkpoint string   `json:"checkpoint,omitempty"`
	Hashes     []string `json:"hashes,omitempty"`
	LogIndex   int64    `json:"log_index"`
	RootHash   string   `json:"root_hash,omitempty"`
	TreeSize   int64    `json:"tree_size"`
}

// TransparencyOptions controls how Selo logs a receipt.
type TransparencyOptions struct {
	LogURL string
	// Soft degrades an upload failure to a recorded "absent" record instead of
	// failing closed. ADR-006 requires that the degraded state is recorded on
	// the receipt — never a predicate that claims a log entry it does not have.
	Soft bool
}

// rekorEntry is the log's response shape for one entry.
type rekorEntry struct {
	Body           string `json:"body"`
	IntegratedTime int64  `json:"integratedTime"`
	LogID          string `json:"logID"`
	LogIndex       int64  `json:"logIndex"`
	Verification   struct {
		SignedEntryTimestamp string `json:"signedEntryTimestamp"`
		InclusionProof       struct {
			Checkpoint string   `json:"checkpoint"`
			Hashes     []string `json:"hashes"`
			LogIndex   int64    `json:"logIndex"`
			RootHash   string   `json:"rootHash"`
			TreeSize   int64    `json:"treeSize"`
		} `json:"inclusionProof"`
	} `json:"verification"`
}

// pemPublicKeyFromB64 converts a base64 raw Ed25519 public key into PEM, which
// is what a Rekor dsse entry's verifier list expects (base64 of the PEM text).
func pemPublicKeyFromB64(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return "", fmt.Errorf("public key base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("a Rekor entry needs an Ed25519 verifier key (got %d bytes)", len(raw))
	}
	der, err := x509.MarshalPKIXPublicKey(ed25519.PublicKey(raw))
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// UploadToRekor logs the receipt's canonical hash in a Rekor transparency log
// and returns the record to store on the receipt.
//
// The entry is a DSSE envelope over the receipt hash, signed by the receipt's
// signer, so the log binds the hash to the same key that signed the receipt.
// Rekor validates the envelope signature before accepting it. On failure it
// returns an error unless opts.Soft is set, in which case it returns a record
// with Status "absent" and a reason — never a silent omission (ADR-006).
func UploadToRekor(canonicalHashHex string, signer Signer, opts TransparencyOptions) (*TransparencyAnchor, error) {
	// The logged payload is the receipt hash itself — nothing else leaves the
	// machine. Rekor stores only its hash, so even this is opaque in the log.
	payload := []byte(canonicalHashHex)
	anc := &TransparencyAnchor{LogURL: opts.LogURL, PayloadHash: sha256Hex(payload)}
	degrade := func(msg string) (*TransparencyAnchor, error) {
		if !opts.Soft {
			return nil, fmt.Errorf("rekor: %s", msg)
		}
		anc.Status = TransparencyAbsent
		anc.Reason = msg
		return anc, nil
	}

	logURL := strings.TrimRight(strings.TrimSpace(opts.LogURL), "/")
	if logURL == "" {
		return degrade("no Rekor URL configured")
	}
	if !strings.HasPrefix(logURL, "http://") && !strings.HasPrefix(logURL, "https://") {
		return degrade(fmt.Sprintf("Rekor URL must be http(s): %q", opts.LogURL))
	}
	pubB64 := signer.PublicKeyB64()
	pubPEM, err := pemPublicKeyFromB64(pubB64)
	if err != nil {
		return degrade(err.Error())
	}

	// The logged payload is the receipt hash itself — nothing else leaves the
	// machine. Rekor stores only its hash, so even this is opaque in the log.
	sig, err := signer.Sign(PAE(DSSEPayloadType, payload))
	if err != nil {
		return degrade(fmt.Sprintf("sign envelope: %v", err))
	}
	env := Envelope{
		PayloadType: DSSEPayloadType,
		Payload:     base64.StdEncoding.EncodeToString(payload),
		Signatures:  []Signature{{Sig: base64.StdEncoding.EncodeToString(sig)}},
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		return degrade(fmt.Sprintf("marshal envelope: %v", err))
	}
	anc.EnvelopeHash = sha256Hex(envJSON)

	body := map[string]any{
		"apiVersion": "0.0.1",
		"kind":       "dsse",
		"spec": map[string]any{
			"proposedContent": map[string]any{
				"envelope":  string(envJSON),
				"verifiers": []string{base64.StdEncoding.EncodeToString([]byte(pubPEM))},
			},
		},
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return degrade(fmt.Sprintf("marshal entry: %v", err))
	}

	httpReq, err := http.NewRequest(http.MethodPost, logURL+"/api/v1/log/entries", bytes.NewReader(reqBody))
	if err != nil {
		return degrade(fmt.Sprintf("build request: %v", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: rekorHTTPTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return degrade(fmt.Sprintf("contact Rekor %s: %v", logURL, err))
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRekorResponseBytes))
	if err != nil {
		return degrade(fmt.Sprintf("read Rekor response: %v", err))
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return degrade(fmt.Sprintf("Rekor returned HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300)))
	}

	var entries map[string]rekorEntry
	if err := json.Unmarshal(respBody, &entries); err != nil {
		return degrade(fmt.Sprintf("parse Rekor response: %v", err))
	}
	if len(entries) == 0 {
		return degrade("Rekor returned no entry")
	}
	for uuid, e := range entries {
		anc.UUID = uuid
		anc.LogID = e.LogID
		anc.LogIndex = e.LogIndex
		anc.IntegratedTime = e.IntegratedTime
		anc.Body = e.Body
		anc.SET = e.Verification.SignedEntryTimestamp
		ip := e.Verification.InclusionProof
		anc.InclusionProof = &RekorInclusionProof{
			Checkpoint: ip.Checkpoint,
			Hashes:     ip.Hashes,
			LogIndex:   ip.LogIndex,
			RootHash:   ip.RootHash,
			TreeSize:   ip.TreeSize,
		}
		break
	}
	anc.Status = TransparencyLogged
	return anc, nil
}

// sha256Hex returns the hex sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// truncate shortens s for an error message.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// hashLeaf is the RFC 6962 leaf hash: sha256(0x00 || data).
func hashLeaf(data []byte) []byte {
	h := sha256.New()
	h.Write([]byte{0x00})
	h.Write(data)
	return h.Sum(nil)
}

// hashChildren is the RFC 6962 interior node hash: sha256(0x01 || left || right).
func hashChildren(left, right []byte) []byte {
	h := sha256.New()
	h.Write([]byte{0x01})
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}

// rootFromInclusionProof walks an RFC 6962 audit path from a leaf hash to the
// tree root. index is the leaf's position, size the tree size at proof time.
func rootFromInclusionProof(leafHash []byte, index, size uint64, proof [][]byte) ([]byte, error) {
	if size == 0 {
		return nil, fmt.Errorf("empty tree")
	}
	if index >= size {
		return nil, fmt.Errorf("leaf index %d out of range for tree of size %d", index, size)
	}
	fn, sn := index, size-1
	r := leafHash
	for _, p := range proof {
		if len(p) != sha256.Size {
			return nil, fmt.Errorf("proof node is %d bytes, want %d", len(p), sha256.Size)
		}
		if fn == sn || fn&1 == 1 {
			r = hashChildren(p, r)
			for fn&1 == 0 && fn != 0 {
				fn >>= 1
				sn >>= 1
			}
		} else {
			r = hashChildren(r, p)
		}
		fn >>= 1
		sn >>= 1
	}
	if sn != 0 {
		return nil, fmt.Errorf("inclusion proof is too short (tree not exhausted)")
	}
	return r, nil
}

// parseCheckpoint splits a signed-note checkpoint into the text that was
// signed, the signer name and signature, and the root hash and tree size it
// commits to.
func parseCheckpoint(cp string) (text, name string, sig []byte, rootHex string, size uint64, err error) {
	i := strings.Index(cp, "\n\n")
	if i < 0 {
		err = fmt.Errorf("checkpoint is not a signed note")
		return
	}
	text = cp[:i+1]
	sigBlock := cp[i+2:]

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) < 3 {
		err = fmt.Errorf("checkpoint has too few lines")
		return
	}
	size, err = strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
	if err != nil {
		err = fmt.Errorf("checkpoint tree size: %w", err)
		return
	}
	rootRaw, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[2]))
	if derr != nil {
		err = fmt.Errorf("checkpoint root hash: %w", derr)
		return
	}
	rootHex = hex.EncodeToString(rootRaw)

	for _, line := range strings.Split(strings.TrimRight(sigBlock, "\n"), "\n") {
		if !strings.HasPrefix(line, "\u2014 ") {
			continue
		}
		rest := strings.TrimPrefix(line, "\u2014 ")
		sp := strings.LastIndex(rest, " ")
		if sp < 0 {
			continue
		}
		raw, derr := base64.StdEncoding.DecodeString(rest[sp+1:])
		// A note signature line is a 4-byte key hash followed by the signature.
		if derr != nil || len(raw) < 5 {
			continue
		}
		name = rest[:sp]
		sig = raw[4:]
		return
	}
	err = fmt.Errorf("checkpoint carries no usable signature")
	return
}

// verifyCheckpointSignature checks a signed-note signature against a log's
// public key (ECDSA/SHA-256, or Ed25519).
func verifyCheckpointSignature(pubPEM, text string, sig []byte) error {
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return fmt.Errorf("log public key is not PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse log public key: %w", err)
	}
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		digest := sha256.Sum256([]byte(text))
		if !ecdsa.VerifyASN1(k, digest[:], sig) {
			return fmt.Errorf("checkpoint signature is invalid")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(k, []byte(text), sig) {
			return fmt.Errorf("checkpoint signature is invalid")
		}
	default:
		return fmt.Errorf("unsupported log public key type %T", pub)
	}
	return nil
}

// VerifyTransparency checks a stored Rekor record against the receipt hash it
// is supposed to be about.
//
// It always checks that the logged payload hash derives from expectedReceiptHash
// (the logged payload is the receipt hash, so the log commits to sha256 of it),
// that the stored entry body still hashes to that value, and that the RFC 6962
// inclusion proof recomputes to the root the checkpoint commits to. When
// logPubKeyPEM is non-empty it additionally verifies the checkpoint's signature
// against that log key — without it the proof is internally consistent but not
// anchored to a trusted log, the same caveat as an unpinned signer.
func VerifyTransparency(t *TransparencyAnchor, expectedReceiptHash, logPubKeyPEM string) error {
	if t == nil {
		return fmt.Errorf("receipt carries no transparency record")
	}
	if t.Status == TransparencyAbsent {
		return fmt.Errorf("transparency record absent: %s", t.Reason)
	}
	if t.PayloadHash == "" {
		return fmt.Errorf("transparency record has no payload hash")
	}
	if expectedReceiptHash != "" && t.PayloadHash != sha256Hex([]byte(expectedReceiptHash)) {
		return fmt.Errorf("transparency record is about a different receipt (payload hash mismatch)")
	}
	if t.InclusionProof == nil {
		return fmt.Errorf("transparency record has no inclusion proof")
	}
	if t.Body == "" {
		return fmt.Errorf("transparency record has no entry body to verify")
	}
	bodyBytes, err := base64.StdEncoding.DecodeString(t.Body)
	if err != nil {
		return fmt.Errorf("entry body base64: %w", err)
	}
	// The canonicalized entry Rekor returned must still commit to the payload
	// hash we recorded; otherwise the stored body is not the entry we logged.
	var logged struct {
		Kind string `json:"kind"`
		Spec struct {
			PayloadHash struct {
				Algorithm string `json:"algorithm"`
				Value     string `json:"value"`
			} `json:"payloadHash"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(bodyBytes, &logged); err != nil {
		return fmt.Errorf("entry body is not JSON: %w", err)
	}
	if v := logged.Spec.PayloadHash.Value; v != "" && !strings.EqualFold(v, t.PayloadHash) {
		return fmt.Errorf("entry body commits to payload hash %s, not the recorded %s", v, t.PayloadHash)
	}

	ip := t.InclusionProof
	proof := make([][]byte, 0, len(ip.Hashes))
	for _, h := range ip.Hashes {
		b, derr := hex.DecodeString(h)
		if derr != nil {
			return fmt.Errorf("inclusion proof node: %w", derr)
		}
		proof = append(proof, b)
	}
	root, err := rootFromInclusionProof(hashLeaf(bodyBytes), uint64(ip.LogIndex), uint64(ip.TreeSize), proof)
	if err != nil {
		return fmt.Errorf("inclusion proof: %w", err)
	}
	if ip.RootHash != "" && !strings.EqualFold(hex.EncodeToString(root), ip.RootHash) {
		return fmt.Errorf("inclusion proof does not lead to the recorded root hash")
	}

	// The checkpoint must commit to the same root and size the proof reached.
	text, _, sig, cpRootHex, cpSize, err := parseCheckpoint(ip.Checkpoint)
	if err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	if !strings.EqualFold(cpRootHex, hex.EncodeToString(root)) {
		return fmt.Errorf("checkpoint root does not match the inclusion proof")
	}
	if cpSize != uint64(ip.TreeSize) {
		return fmt.Errorf("checkpoint tree size %d does not match the proof's %d", cpSize, ip.TreeSize)
	}
	if logPubKeyPEM == "" {
		return nil
	}
	return verifyCheckpointSignature(logPubKeyPEM, text, sig)
}

// LoadRekorPublicKey reads a PEM-encoded Rekor log public key from path.
func LoadRekorPublicKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Rekor public key: %w", err)
	}
	if block, _ := pem.Decode(data); block == nil {
		return "", fmt.Errorf("no PEM public key found in %s", path)
	}
	return string(data), nil
}

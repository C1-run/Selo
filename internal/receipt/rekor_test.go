package receipt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- test helpers: a local Rekor-compatible log --------------------------

// buildTestTree builds an RFC 6962 tree over the given leaves and returns its
// root plus the audit path for leaf idx. It assumes a power-of-two leaf count,
// which is enough for the fixtures here.
func buildTestTree(leaves [][]byte, idx int) ([]byte, [][]byte) {
	level := make([][]byte, len(leaves))
	for i, l := range leaves {
		level[i] = hashLeaf(l)
	}
	var proof [][]byte
	pos := idx
	for len(level) > 1 {
		if sib := pos ^ 1; sib < len(level) {
			proof = append(proof, level[sib])
		}
		next := make([][]byte, 0, len(level)/2)
		for i := 0; i+1 < len(level); i += 2 {
			next = append(next, hashChildren(level[i], level[i+1]))
		}
		level = next
		pos >>= 1
	}
	return level[0], proof
}

// makeCheckpoint builds a signed-note checkpoint over root/size, signed with an
// ECDSA/SHA-256 key — the same shape the public Rekor log publishes.
func makeCheckpoint(name string, root []byte, size uint64, key *ecdsa.PrivateKey) string {
	text := fmt.Sprintf("%s - %d\n%d\n%s\n", name, 1, size, base64.StdEncoding.EncodeToString(root))
	digest := sha256.Sum256([]byte(text))
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		panic(err)
	}
	blob := append([]byte{0xde, 0xad, 0xbe, 0xef}, sig...) // 4-byte key hash + signature
	return text + "\n\u2014 " + name + " " + base64.StdEncoding.EncodeToString(blob) + "\n"
}

func pemOfPublicKey(t *testing.T, pub any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// testEntryBody renders the canonicalized Rekor entry body for a payload.
func testEntryBody(payload []byte) string {
	return fmt.Sprintf(`{"apiVersion":"0.0.1","kind":"dsse","spec":{"payloadHash":{"algorithm":"sha256","value":"%s"}}}`,
		sha256Hex(payload))
}

func newUploadSigner(t *testing.T) Signer {
	t.Helper()
	_, pubB64, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &localSigner{priv: priv, pubB64: pubB64, mode: KeyModePersistent, source: KeySourceFile}
}

// rekorFixture is a stand-in for a Rekor log: it accepts a dsse entry, puts it
// at a fixed position in a local Merkle tree, and answers with a real inclusion
// proof and a signed checkpoint.
type rekorFixture struct {
	logKey    *ecdsa.PrivateKey
	logPubPEM string
	index     int
	size      int
	leafCount int
}

func newRekorFixture(t *testing.T) *rekorFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate log key: %v", err)
	}
	return &rekorFixture{logKey: key, logPubPEM: pemOfPublicKey(t, &key.PublicKey), index: 3, size: 8, leafCount: 8}
}

func (f *rekorFixture) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/log/entries" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Kind string `json:"kind"`
			Spec struct {
				ProposedContent struct {
					Envelope string `json:"envelope"`
				} `json:"proposedContent"`
			} `json:"spec"`
		}
		if json.Unmarshal(raw, &req) != nil || req.Kind != "dsse" || req.Spec.ProposedContent.Envelope == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var env Envelope
		if json.Unmarshal([]byte(req.Spec.ProposedContent.Envelope), &env) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payload, err := base64.StdEncoding.DecodeString(env.Payload)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Place our entry at f.index among f.leafCount leaves.
		leaves := make([][]byte, f.leafCount)
		for i := range leaves {
			if i == f.index {
				leaves[i] = []byte(testEntryBody(payload))
			} else {
				leaves[i] = []byte(fmt.Sprintf("filler-%d", i))
			}
		}
		root, proof := buildTestTree(leaves, f.index)
		hashes := make([]string, 0, len(proof))
		for _, p := range proof {
			hashes = append(hashes, hex.EncodeToString(p))
		}
		resp := map[string]any{
			"deadbeefcafe": map[string]any{
				"body":           base64.StdEncoding.EncodeToString(leaves[f.index]),
				"integratedTime": int64(1700000000),
				"logID":          "d32f30a3c32d639c",
				"logIndex":       f.index,
				"verification": map[string]any{
					"signedEntryTimestamp": "c2V0",
					"inclusionProof": map[string]any{
						"checkpoint": makeCheckpoint("rekor.test", root, uint64(f.size), f.logKey),
						"hashes":     hashes,
						"logIndex":   f.index,
						"rootHash":   hex.EncodeToString(root),
						"treeSize":   f.size,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// --- tests ---------------------------------------------------------------

func TestUploadToRekorRoundTrip(t *testing.T) {
	f := newRekorFixture(t)
	srv := f.server(t)
	defer srv.Close()

	signer := newUploadSigner(t)
	receiptHash := sha256Hex([]byte(`{"receipt_id":"c1f-1"}`))

	anc, err := UploadToRekor(receiptHash, signer, TransparencyOptions{LogURL: srv.URL})
	if err != nil {
		t.Fatalf("UploadToRekor: %v", err)
	}
	if anc.Status != TransparencyLogged {
		t.Fatalf("status = %q, want %q (reason %q)", anc.Status, TransparencyLogged, anc.Reason)
	}
	if anc.LogIndex != int64(f.index) || anc.IntegratedTime != 1700000000 {
		t.Errorf("log index/time = %d/%d, want %d/1700000000", anc.LogIndex, anc.IntegratedTime, f.index)
	}
	if anc.UUID == "" || anc.LogID == "" || anc.Body == "" || anc.SET == "" {
		t.Errorf("record incomplete: uuid=%q logID=%q body=%d set=%q", anc.UUID, anc.LogID, len(anc.Body), anc.SET)
	}
	if anc.InclusionProof == nil || len(anc.InclusionProof.Hashes) == 0 {
		t.Fatal("no inclusion proof recorded")
	}
	if want := sha256Hex([]byte(receiptHash)); anc.PayloadHash != want {
		t.Errorf("payload hash = %s, want %s", anc.PayloadHash, want)
	}
	// The whole point: the stored record verifies against the log key.
	if err := VerifyTransparency(anc, receiptHash, f.logPubPEM); err != nil {
		t.Fatalf("VerifyTransparency: %v", err)
	}
	// Without the log key it is only self-consistent, but still consistent.
	if err := VerifyTransparency(anc, receiptHash, ""); err != nil {
		t.Fatalf("VerifyTransparency without a log key: %v", err)
	}
}

func TestVerifyTransparencyRejectsWrongLogKey(t *testing.T) {
	f := newRekorFixture(t)
	srv := f.server(t)
	defer srv.Close()

	signer := newUploadSigner(t)
	rh := sha256Hex([]byte("payload"))
	anc, err := UploadToRekor(rh, signer, TransparencyOptions{LogURL: srv.URL})
	if err != nil {
		t.Fatalf("UploadToRekor: %v", err)
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err := VerifyTransparency(anc, rh, pemOfPublicKey(t, &other.PublicKey)); err == nil {
		t.Fatal("expected verification to fail against an unrelated log key")
	}
}

func TestVerifyTransparencyRejectsPayloadMismatch(t *testing.T) {
	f := newRekorFixture(t)
	srv := f.server(t)
	defer srv.Close()

	signer := newUploadSigner(t)
	rh := sha256Hex([]byte("payload"))
	anc, err := UploadToRekor(rh, signer, TransparencyOptions{LogURL: srv.URL})
	if err != nil {
		t.Fatalf("UploadToRekor: %v", err)
	}
	if err := VerifyTransparency(anc, sha256Hex([]byte("a different receipt")), f.logPubPEM); err == nil {
		t.Fatal("expected verification to fail when the payload hash differs")
	}
}

func TestVerifyTransparencyRejectsTamperedBody(t *testing.T) {
	f := newRekorFixture(t)
	srv := f.server(t)
	defer srv.Close()

	signer := newUploadSigner(t)
	rh := sha256Hex([]byte("payload"))
	anc, err := UploadToRekor(rh, signer, TransparencyOptions{LogURL: srv.URL})
	if err != nil {
		t.Fatalf("UploadToRekor: %v", err)
	}
	// Flip the entry body: the leaf hash no longer reaches the checkpoint root.
	body, _ := base64.StdEncoding.DecodeString(anc.Body)
	body[len(body)-2] ^= 0x01
	anc.Body = base64.StdEncoding.EncodeToString(body)
	if err := VerifyTransparency(anc, rh, f.logPubPEM); err == nil {
		t.Fatal("expected verification to fail on a tampered entry body")
	}
}

func TestVerifyTransparencyRejectsTamperedProof(t *testing.T) {
	f := newRekorFixture(t)
	srv := f.server(t)
	defer srv.Close()

	signer := newUploadSigner(t)
	rh := sha256Hex([]byte("payload"))
	anc, err := UploadToRekor(rh, signer, TransparencyOptions{LogURL: srv.URL})
	if err != nil {
		t.Fatalf("UploadToRekor: %v", err)
	}
	// Corrupt one audit-path node.
	anc.InclusionProof.Hashes[0] = "00" + anc.InclusionProof.Hashes[0][2:]
	if err := VerifyTransparency(anc, rh, f.logPubPEM); err == nil {
		t.Fatal("expected verification to fail on a corrupted inclusion proof")
	}
}

func TestUploadToRekorSoftOnFailure(t *testing.T) {
	signer := newUploadSigner(t)
	rh := sha256Hex([]byte("payload"))

	// Fail closed by default.
	if _, err := UploadToRekor(rh, signer, TransparencyOptions{LogURL: "http://127.0.0.1:1/"}); err == nil {
		t.Fatal("expected an error for an unreachable log")
	}
	anc, err := UploadToRekor(rh, signer, TransparencyOptions{LogURL: "http://127.0.0.1:1/", Soft: true})
	if err != nil {
		t.Fatalf("soft mode should not error: %v", err)
	}
	if anc.Status != TransparencyAbsent || anc.Reason == "" {
		t.Fatalf("soft mode should record an absence with a reason, got status=%q reason=%q", anc.Status, anc.Reason)
	}
	if err := VerifyTransparency(anc, rh, ""); err == nil {
		t.Fatal("verifying an absent record must fail")
	}
}

func TestUploadToRekorRejectsNonHTTPURL(t *testing.T) {
	signer := newUploadSigner(t)
	if _, err := UploadToRekor("abc", signer, TransparencyOptions{LogURL: "ftp://rekor.example"}); err == nil {
		t.Fatal("expected a non-http(s) log URL to be rejected")
	}
	if _, err := UploadToRekor("abc", signer, TransparencyOptions{}); err == nil {
		t.Fatal("expected an empty log URL to be rejected")
	}
}

// TestTransparencyNotCoveredBySignature guards the design invariant: the log
// record is produced after signing and must not alter the canonical bytes.
func TestTransparencyNotCoveredBySignature(t *testing.T) {
	r := &ForgeReceipt{ReceiptID: "c1f-1", TaskID: "t1", Verdict: VerdictSuccess}
	before, err := CanonicalJSON(r)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	r.Transparency = &TransparencyAnchor{
		LogURL:      "https://rekor.sigstore.dev",
		LogIndex:    42,
		PayloadHash: sha256Hex(before),
		Status:      TransparencyLogged,
	}
	after, err := CanonicalJSON(r)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("adding a transparency record changed the canonical JSON; the signature would no longer verify")
	}
}

package receipt

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
)

// testTSA is a self-contained RFC3161 authority for tests: it holds a
// self-signed certificate with the timestamping EKU and signs TSTInfo
// structures with the library's server-side helper. Nothing here touches the
// network, so the tests are hermetic.
type testTSA struct {
	cert *x509.Certificate
	priv *rsa.PrivateKey
	// imprintOverride, when non-nil, is imprinted instead of the requested
	// digest — used to prove Selo rejects a token about different data.
	imprintOverride []byte
	now             time.Time
}

func newTestTSA(t *testing.T) *testTSA {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate TSA key: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Selo Test TSA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create TSA cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse TSA cert: %v", err)
	}
	return &testTSA{cert: cert, priv: priv, now: now}
}

// roots returns a pool trusting this TSA.
func (s *testTSA) roots() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(s.cert)
	return pool
}

// responseFor builds a DER TimeStampResp for the given message imprint.
func (s *testTSA) responseFor(t *testing.T, hashAlg crypto.Hash, imprint []byte) []byte {
	t.Helper()
	ts := &timestamp.Timestamp{
		HashAlgorithm:     hashAlg,
		HashedMessage:     imprint,
		Time:              s.now,
		Policy:            asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1},
		AddTSACertificate: true,
	}
	resp, err := ts.CreateResponseWithOpts(s.cert, s.priv, crypto.SHA256)
	if err != nil {
		t.Fatalf("create TSA response: %v", err)
	}
	return resp
}

// server returns an httptest server that honours timestamp requests. A bad
// request (unparseable) yields 400.
func (s *testTSA) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req, err := timestamp.ParseRequest(body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		imprint := req.HashedMessage
		if s.imprintOverride != nil {
			imprint = s.imprintOverride
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		w.Write(s.responseFor(t, req.HashAlgorithm, imprint))
	}))
}

func TestTimestampCanonicalRoundTrip(t *testing.T) {
	tsa := newTestTSA(t)
	srv := tsa.server(t)
	defer srv.Close()

	canonical := []byte(`{"receipt_id":"c1f-1","verdict":"SUCCESS_WITH_RECEIPT"}`)
	anc, err := TimestampCanonical(canonical, TimestampOptions{TSAURL: srv.URL})
	if err != nil {
		t.Fatalf("TimestampCanonical: %v", err)
	}
	if anc.Status != TimestampGranted {
		t.Fatalf("status = %q, want %q (reason %q)", anc.Status, TimestampGranted, anc.Reason)
	}
	if anc.Token == "" {
		t.Fatal("no token recorded")
	}
	sum := sha256.Sum256(canonical)
	if anc.Digest != hex.EncodeToString(sum[:]) {
		t.Errorf("digest = %s, want %s", anc.Digest, hex.EncodeToString(sum[:]))
	}
	if !anc.GenTime.Equal(tsa.now) {
		t.Errorf("gen_time = %v, want %v", anc.GenTime, tsa.now)
	}
	if anc.Policy == "" {
		t.Error("policy not recorded")
	}

	// A token obtained from the TSA must verify against the receipt digest...
	if err := VerifyTimestamp(anc, anc.Digest, tsa.roots()); err != nil {
		t.Errorf("VerifyTimestamp with trusted roots: %v", err)
	}
	// ...and also without roots, as self-consistency only.
	if err := VerifyTimestamp(anc, anc.Digest, nil); err != nil {
		t.Errorf("VerifyTimestamp without roots: %v", err)
	}
}

func TestVerifyTimestampRejectsUntrustedRoot(t *testing.T) {
	tsa := newTestTSA(t)
	srv := tsa.server(t)
	defer srv.Close()

	anc, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: srv.URL})
	if err != nil {
		t.Fatalf("TimestampCanonical: %v", err)
	}
	// A different, unrelated TSA is not a trust anchor for this token.
	other := newTestTSA(t)
	if err := VerifyTimestamp(anc, anc.Digest, other.roots()); err == nil {
		t.Fatal("expected verification to fail against an unrelated root")
	}
	// Without roots we can only assert self-consistency, which holds.
	if err := VerifyTimestamp(anc, anc.Digest, nil); err != nil {
		t.Fatalf("self-consistency check should pass, got: %v", err)
	}
}

func TestVerifyTimestampRejectsImprintMismatch(t *testing.T) {
	tsa := newTestTSA(t)
	srv := tsa.server(t)
	defer srv.Close()

	anc, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: srv.URL})
	if err != nil {
		t.Fatalf("TimestampCanonical: %v", err)
	}
	// Claim the timestamp is over a different receipt hash.
	bogus := sha256.Sum256([]byte("some other receipt"))
	if err := VerifyTimestamp(anc, hex.EncodeToString(bogus[:]), tsa.roots()); err == nil {
		t.Fatal("expected verification to fail when the imprint does not match")
	}
}

func TestVerifyTimestampRejectsTamperedToken(t *testing.T) {
	tsa := newTestTSA(t)
	srv := tsa.server(t)
	defer srv.Close()

	anc, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: srv.URL})
	if err != nil {
		t.Fatalf("TimestampCanonical: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(anc.Token)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	// Flip a byte in the signature region; the CMS signature check must fail.
	raw[len(raw)-1] ^= 0xff
	anc.Token = base64.StdEncoding.EncodeToString(raw)
	if err := VerifyTimestamp(anc, anc.Digest, tsa.roots()); err == nil {
		t.Fatal("expected verification to fail on a tampered token")
	}
}

func TestTimestampCanonicalRejectsMismatchedImprint(t *testing.T) {
	tsa := newTestTSA(t)
	tsa.imprintOverride = sha256sum([]byte("not the payload"))
	srv := tsa.server(t)
	defer srv.Close()

	// Fail closed by default: a token about other data must not be recorded.
	if _, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: srv.URL}); err == nil {
		t.Fatal("expected an error when the TSA imprints a different digest")
	}
	// With Soft it degrades to a recorded absence, never a false grant.
	anc, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: srv.URL, Soft: true})
	if err != nil {
		t.Fatalf("soft mode should not error: %v", err)
	}
	if anc.Status != TimestampAbsent || anc.Reason == "" {
		t.Fatalf("soft mode should record an absence with a reason, got status=%q reason=%q", anc.Status, anc.Reason)
	}
}

func TestTimestampCanonicalSoftOnUnreachableTSA(t *testing.T) {
	// Fail closed without Soft.
	if _, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: "http://127.0.0.1:1/"}); err == nil {
		t.Fatal("expected an error for an unreachable TSA")
	}
	anc, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: "http://127.0.0.1:1/", Soft: true})
	if err != nil {
		t.Fatalf("soft mode should not error: %v", err)
	}
	if anc.Status != TimestampAbsent {
		t.Fatalf("status = %q, want %q", anc.Status, TimestampAbsent)
	}
	if err := VerifyTimestamp(anc, anc.Digest, nil); err == nil {
		t.Fatal("verifying an absent timestamp must fail")
	}
}

func TestTimestampCanonicalRejectsNonHTTPURL(t *testing.T) {
	if _, err := TimestampCanonical([]byte("payload"), TimestampOptions{TSAURL: "ftp://tsa.example"}); err == nil {
		t.Fatal("expected a non-http(s) TSA URL to be rejected")
	}
	if _, err := TimestampCanonical([]byte("payload"), TimestampOptions{}); err == nil {
		t.Fatal("expected an empty TSA URL to be rejected")
	}
}

// --- small helpers -------------------------------------------------------

func sha256sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

// TestTimestampNotCoveredBySignature guards the design invariant: the timestamp
// is produced after signing and must not alter the canonical bytes (otherwise
// adding a timestamp would invalidate the signature).
func TestTimestampNotCoveredBySignature(t *testing.T) {
	r := &ForgeReceipt{ReceiptID: "c1f-1", TaskID: "t1", Verdict: VerdictSuccess}
	before, err := CanonicalJSON(r)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	r.Timestamp = &TimestampAnchor{
		TSAURL: "https://tsa.example",
		Token:  base64.StdEncoding.EncodeToString([]byte("token")),
		Digest: hex.EncodeToString(sha256sum(before)),
		Status: TimestampGranted,
	}
	after, err := CanonicalJSON(r)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("adding a timestamp changed the canonical JSON; the signature would no longer verify")
	}
}

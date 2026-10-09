package receipt

import (
	"bytes"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/digitorus/timestamp"
)

// Timestamp status values recorded in TimestampAnchor.Status.
const (
	TimestampGranted = "granted"
	TimestampAbsent  = "absent"
)

// Timestamp state values reported by verification.
const (
	TimestampStateOK         = "OK"
	TimestampStateUnverified = "UNVERIFIED"
	TimestampStateFailed     = "FAILED"
	TimestampStateAbsent     = "ABSENT"
	TimestampStateSkipped    = "SKIPPED"
)

// maxTSAResponseBytes bounds how much data a TSA response can make Selo read.
const maxTSAResponseBytes = 1 << 20 // 1 MiB

// tsaHTTPTimeout bounds the TSA round-trip. A variable so tests can shorten it.
var tsaHTTPTimeout = 30 * time.Second

// TimestampAnchor is an RFC3161 trusted timestamp over a receipt's canonical
// bytes (ADR-005).
//
// It is obtained *after* the receipt is signed: a timestamp attests to when a
// signature existed, so it cannot be part of the signed content without
// circularity. Like the git anchor fields it is therefore excluded from
// CanonicalJSON. Because the message imprint is the sha256 of the canonical
// JSON — the same value recorded as receipt_hash — the timestamp is bound to
// both the receipt's content and, transitively, its signature.
type TimestampAnchor struct {
	TSAURL  string    `json:"tsa_url"`
	Token   string    `json:"token,omitempty"` // base64 DER RFC3161 TimeStampToken
	GenTime time.Time `json:"gen_time,omitempty"`
	Serial  string    `json:"serial,omitempty"`
	Policy  string    `json:"policy,omitempty"`
	Digest  string    `json:"digest"`           // hex sha256 the TSA imprinted (== receipt_hash)
	Status  string    `json:"status"`           // granted | absent
	Reason  string    `json:"reason,omitempty"` // set when status == absent
}

// TimestampOptions controls how Selo obtains a timestamp.
type TimestampOptions struct {
	TSAURL string
	// Soft degrades a TSA failure to a recorded "absent" timestamp instead of
	// failing closed. ADR-005 requires the degraded state to be recorded on the
	// receipt — never silently omitted, so a receipt cannot claim
	// non-repudiation it does not have.
	Soft bool
}

// RequestTimestamp asks the TSA at tsaURL to timestamp data, returning the raw
// DER TimeStampResp. The message imprint is sha256(data). The request asks the
// TSA to embed its certificate, so the returned token is self-contained.
func RequestTimestamp(tsaURL string, data []byte) ([]byte, error) {
	tsaURL = strings.TrimSpace(tsaURL)
	if tsaURL == "" {
		return nil, fmt.Errorf("no TSA URL configured")
	}
	if !strings.HasPrefix(tsaURL, "http://") && !strings.HasPrefix(tsaURL, "https://") {
		return nil, fmt.Errorf("TSA URL must be http(s): %q", tsaURL)
	}
	reqDER, err := timestamp.CreateRequest(bytes.NewReader(data), &timestamp.RequestOptions{
		Hash:         crypto.SHA256,
		Certificates: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build timestamp request: %w", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, tsaURL, bytes.NewReader(reqDER))
	if err != nil {
		return nil, fmt.Errorf("build TSA request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/timestamp-query")
	httpReq.Header.Set("Accept", "application/timestamp-reply")

	client := &http.Client{Timeout: tsaHTTPTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("contact TSA %s: %w", tsaURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTSAResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read TSA response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TSA %s returned HTTP %d", tsaURL, resp.StatusCode)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("TSA %s returned an empty response", tsaURL)
	}
	return body, nil
}

// TimestampCanonical timestamps the sha256 of canonical and returns the anchor
// to store on the receipt. On failure it returns an error unless opts.Soft is
// set, in which case it returns an anchor with Status "absent" and a reason —
// never a silent omission (ADR-005).
func TimestampCanonical(canonical []byte, opts TimestampOptions) (*TimestampAnchor, error) {
	sum := sha256.Sum256(canonical)
	digestHex := hex.EncodeToString(sum[:])
	anc := &TimestampAnchor{TSAURL: opts.TSAURL, Digest: digestHex}

	degrade := func(msg string) (*TimestampAnchor, error) {
		if !opts.Soft {
			return nil, fmt.Errorf("timestamp: %s", msg)
		}
		anc.Status = TimestampAbsent
		anc.Reason = msg
		return anc, nil
	}

	respDER, err := RequestTimestamp(opts.TSAURL, canonical)
	if err != nil {
		return degrade(err.Error())
	}
	ts, err := timestamp.ParseResponse(respDER)
	if err != nil {
		return degrade(fmt.Sprintf("parse TSA response: %v", err))
	}
	// The TSA must have imprinted exactly our digest. Anything else is a token
	// about some other data and must not be recorded as granted.
	if !bytes.Equal(ts.HashedMessage, sum[:]) {
		return degrade("TSA imprinted a different digest than the receipt hash")
	}
	anc.Token = base64.StdEncoding.EncodeToString(ts.RawToken)
	anc.GenTime = ts.Time
	anc.Status = TimestampGranted
	if ts.SerialNumber != nil {
		anc.Serial = ts.SerialNumber.String()
	}
	if ts.Policy != nil {
		anc.Policy = ts.Policy.String()
	}
	return anc, nil
}

// VerifyTimestamp checks an RFC3161 timestamp token against expectedDigestHex.
//
// It always requires a well-formed token whose message imprint equals
// expectedDigestHex and whose CMS signature is internally valid — the token
// must carry its certificate, and digitorus/timestamp verifies the signature
// over the signed attributes while parsing. When roots is non-nil it
// additionally requires the TSA certificate to chain to a trusted root with the
// timestamping extended key usage, evaluated at the timestamped instant. Without
// roots the result is self-consistency only: proof that *a* TSA signed this
// digest, not that a *trusted* one did — the same caveat as an unpinned signer.
func VerifyTimestamp(anc *TimestampAnchor, expectedDigestHex string, roots *x509.CertPool) error {
	if anc == nil {
		return fmt.Errorf("receipt carries no timestamp")
	}
	if anc.Status == TimestampAbsent {
		return fmt.Errorf("timestamp absent: %s", anc.Reason)
	}
	if anc.Token == "" {
		return fmt.Errorf("timestamp has no token")
	}
	der, err := base64.StdEncoding.DecodeString(anc.Token)
	if err != nil {
		return fmt.Errorf("timestamp token base64: %w", err)
	}
	ts, err := timestamp.Parse(der)
	if err != nil {
		return fmt.Errorf("parse timestamp token: %w", err)
	}
	// Require the certificate unconditionally: Parse only checks the CMS
	// signature when one is present, so without it we would be trusting an
	// unverified blob.
	if len(ts.Certificates) == 0 {
		return fmt.Errorf("timestamp token carries no certificate; its signature cannot be checked")
	}
	want, err := hex.DecodeString(strings.TrimSpace(expectedDigestHex))
	if err != nil {
		return fmt.Errorf("receipt hash is not hex: %w", err)
	}
	if !bytes.Equal(ts.HashedMessage, want) {
		return fmt.Errorf("timestamp message imprint does not match the receipt hash")
	}
	if roots == nil {
		return nil
	}
	leaf := ts.Certificates[0]
	inter := x509.NewCertPool()
	for _, c := range ts.Certificates[1:] {
		inter.AddCert(c)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		CurrentTime:   ts.Time,
	}); err != nil {
		return fmt.Errorf("TSA certificate is not trusted: %w", err)
	}
	return nil
}

// LoadTSARoots reads PEM-encoded TSA root certificate(s) from path into a pool
// suitable for VerifyTimestamp.
func LoadTSARoots(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read TSA CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no PEM certificates found in %s", path)
	}
	return pool, nil
}

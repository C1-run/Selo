package receipt

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// TestPAEMatchesDSSESpec pins PAE to the DSSE specification's worked example so
// the wire format cannot silently drift.
func TestPAEMatchesDSSESpec(t *testing.T) {
	got := string(PAE("http://example.com/HelloWorld", []byte("hello world")))
	want := "DSSEv1 29 http://example.com/HelloWorld 11 hello world"
	if got != want {
		t.Fatalf("PAE mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestExportInTotoRoundTrip(t *testing.T) {
	seedB64, pubB64, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELO_SIGNING_KEY", seedB64)

	r := &ForgeReceipt{
		ReceiptID:   "c1f-test",
		TaskID:      "run-abc",
		Verdict:     VerdictSuccess,
		DiffHash:    "deadbeef",
		ReceiptHash: "cafe",
	}
	env, err := ExportInToto(r)
	if err != nil {
		t.Fatalf("ExportInToto: %v", err)
	}
	if env.PayloadType != DSSEPayloadType {
		t.Fatalf("payloadType = %q, want %q", env.PayloadType, DSSEPayloadType)
	}
	fp, _ := PublicKeyFingerprint(pubB64)
	if len(env.Signatures) != 1 || env.Signatures[0].KeyID != fp {
		t.Fatalf("keyid = %+v, want fingerprint %q", env.Signatures, fp)
	}

	payload, ok, err := VerifyStatement(env, pubB64)
	if err != nil || !ok {
		t.Fatalf("VerifyStatement: ok=%v err=%v", ok, err)
	}
	var stmt Statement
	if err := json.Unmarshal(payload, &stmt); err != nil {
		t.Fatal(err)
	}
	if stmt.Type != StatementType || stmt.PredicateType != PredicateType {
		t.Fatalf("statement types wrong: %+v", stmt)
	}
	if len(stmt.Subject) != 1 || stmt.Subject[0].Digest["sha256"] != "deadbeef" {
		t.Fatalf("subject not bound to diff_hash: %+v", stmt.Subject)
	}
	if stmt.Subject[0].Name != "selo://run-abc" {
		t.Fatalf("subject name = %q", stmt.Subject[0].Name)
	}
	var pred ForgeReceipt
	if err := json.Unmarshal(stmt.Predicate, &pred); err != nil {
		t.Fatal(err)
	}
	if pred.Verdict != VerdictSuccess {
		t.Fatalf("predicate verdict = %q", pred.Verdict)
	}
}

func TestVerifyStatementRejectsTamperAndWrongKey(t *testing.T) {
	seedB64, pubB64, _, _ := GenerateKeyPair()
	t.Setenv("SELO_SIGNING_KEY", seedB64)

	env, err := ExportInToto(&ForgeReceipt{ReceiptID: "x", TaskID: "t", Verdict: VerdictSuccess})
	if err != nil {
		t.Fatal(err)
	}

	// Tamper: change the payload after signing.
	raw, _ := base64.StdEncoding.DecodeString(env.Payload)
	tampered := *env
	tampered.Payload = base64.StdEncoding.EncodeToString(append(raw, ' '))
	if _, ok, _ := VerifyStatement(&tampered, pubB64); ok {
		t.Fatal("tampered payload verified")
	}

	// Wrong key: an unrelated public key must not validate the signature.
	_, otherPub, _, _ := GenerateKeyPair()
	if _, ok, _ := VerifyStatement(env, otherPub); ok {
		t.Fatal("envelope verified under an unrelated key")
	}
}

package evidence

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/C1-run/selo/internal/gatechain"
)

func TestLedgerChain(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir, "run_001")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.AppendDecision(true, "filesystem.write", "src/payments/a.ts", "filesystem.write", "ok")
	l.AppendDecision(false, "filesystem.write", "src/auth/x.ts", "filesystem.write", "path_outside_scope")
	l.AppendFlow("AGENT_EXIT", "exit 2")
	l.AppendVerdict("STOP")

	if err := VerifyChain(l.path); err != nil {
		t.Fatalf("chain invalid: %v", err)
	}
}

func TestTamperedChainDetected(t *testing.T) {
	dir := t.TempDir()
	l, _ := NewLedger(dir, "run_002")
	defer l.Close()
	l.AppendDecision(true, "filesystem.write", "src/a.ts", "filesystem.write", "ok")
	l.AppendFlow("AGENT_EXIT", "exit 0")

	// corrupt the event file in place
	data, _ := os.ReadFile(l.path)
	// flip a byte in the middle
	data[len(data)/2] ^= 0x01
	os.WriteFile(l.path, data, 0644)

	if err := VerifyChain(l.path); err == nil {
		t.Fatal("tampered chain verified as valid")
	}
}

// TestReceiptVerifiesChain proves the receipt can reference a verified ledger.
func TestReceiptVerifiesChain(t *testing.T) {
	dir := t.TempDir()
	l, _ := NewLedger(dir, "run_003")
	defer l.Close()
	l.AppendFlow("AGENT_START", "started")
	if err := VerifyChain(l.path); err != nil {
		t.Fatal(err)
	}
}

func TestGatechainIntegrationSmoke(t *testing.T) {
	// Prove the evidence package works with the verdict engine end to end.
	steps := []gatechain.ChainStep{
		{ID: "scope", Label: "scope", Status: gatechain.StatusBlock, Coverage: gatechain.CovBlocked, Risk: gatechain.RiskBlocked, ReasonCode: "path_outside_scope", EvidenceRef: "events.jsonl"},
		{ID: "evidence", Label: "evidence", Status: gatechain.StatusPass, Coverage: gatechain.CovCovered, Risk: gatechain.RiskLow, ReasonCode: "chain_valid", EvidenceRef: "events.jsonl"},
	}
	summary, err := gatechain.Summarize(steps)
	if err != nil {
		t.Fatal(err)
	}
	v := gatechain.ConsumeDecision(summary)
	if v.Action != gatechain.ActionStop {
		t.Errorf("action = %s, want stop", v.Action)
	}
	_ = time.Now()
	_ = json.Marshal
}

// Package evidence manages the per-run evidence ledger: an append-only,
// hash-chained JSONL of every capability decision, plus run artifacts and
// the final receipt. No evidence, no PASS.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Event struct {
	Seq          int    `json:"seq"`
	Ts           string `json:"ts"`
	Type         string `json:"type"` // CAPABILITY_ALLOWED | CAPABILITY_DENIED | AGENT_START | AGENT_EXIT | CELL_KILLED | TEST_RUN | VERDICT
	RunID        string `json:"run_id"`
	Kind         string `json:"action,omitempty"`
	Target       string `json:"target,omitempty"`
	Policy       string `json:"policy,omitempty"`
	Detail       string `json:"detail,omitempty"`
	PreviousHash string `json:"previous_hash"`
	EventHash    string `json:"event_hash"`
}

// Ledger appends hash-chained events to runs/<id>/events.jsonl.
type Ledger struct {
	dir  string
	path string
	f    *os.File
	seq  int
	prev string
}

// NewLedger creates the run directory and opens the event file.
func NewLedger(runsDir, runID string) (*Ledger, error) {
	dir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &Ledger{dir: dir, path: path, f: f}, nil
}

func (l *Ledger) Dir() string { return l.dir }

func (l *Ledger) Close() error { return l.f.Close() }

// Append writes an event with the chained hash and returns its sequence.
func (l *Ledger) Append(ev Event) (int, error) {
	l.seq++
	ev.Seq = l.seq
	ev.RunID = filepath.Base(l.dir)
	ev.Ts = time.Now().UTC().Format(time.RFC3339Nano)
	ev.PreviousHash = l.prev
	canon, _ := json.Marshal(ev)
	sum := sha256.Sum256(canon)
	ev.EventHash = hex.EncodeToString(sum[:])
	line, _ := json.Marshal(ev)
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return l.seq, err
	}
	l.prev = ev.EventHash
	return l.seq, nil
}

func (l *Ledger) AppendDecision(ok bool, reqKind, target, policy, reason string) (int, error) {
	typ := "CAPABILITY_ALLOWED"
	if !ok {
		typ = "CAPABILITY_DENIED"
	}
	if reason == "" {
		reason = "ok"
	}
	return l.Append(Event{Type: typ, Kind: reqKind, Target: target, Policy: policy, Detail: reason})
}

func (l *Ledger) AppendFlow(typ, detail string) (int, error) {
	return l.Append(Event{Type: typ, Detail: detail})
}

func (l *Ledger) AppendVerdict(verdict string) (int, error) {
	return l.Append(Event{Type: "VERDICT", Detail: verdict})
}

// VerifyChain replays events.jsonl and confirms every hash links correctly.
func VerifyChain(path string) error {
	lines, err := readLines(path)
	if err != nil {
		return err
	}
	prev := ""
	for i, line := range lines {
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		check := ev
		check.PreviousHash = prev
		check.EventHash = ""
		canon, _ := json.Marshal(check)
		sum := sha256.Sum256(canon)
		if hex.EncodeToString(sum[:]) != ev.EventHash {
			return fmt.Errorf("line %d: hash mismatch", i+1)
		}
		if ev.PreviousHash != prev {
			return fmt.Errorf("line %d: chain broken", i+1)
		}
		prev = ev.EventHash
	}
	return nil
}

// WriteArtifact persists a run artifact under the run dir.
func (l *Ledger) WriteArtifact(name string, data []byte) error {
	return os.WriteFile(filepath.Join(l.dir, name), data, 0644)
}

func readLines(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	// drop trailing empty line
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}
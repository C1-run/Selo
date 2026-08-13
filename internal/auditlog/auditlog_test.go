package auditlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateAuditEvent(t *testing.T) {
	ev := CreateAuditEvent("c1-forge", "p45_stop_written", "task-001", "scope violation", map[string]string{"raw_content_included": "false"}, "")
	if ev.ID == "" {
		t.Error("expected non-empty ID")
	}
	if ev.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
	if ev.Actor != "c1-forge" {
		t.Errorf("actor = %q, want c1-forge", ev.Actor)
	}
	if ev.Action != "p45_stop_written" {
		t.Errorf("action = %q, want p45_stop_written", ev.Action)
	}
	if ev.Target != "task-001" {
		t.Errorf("target = %q, want task-001", ev.Target)
	}
	if ev.Reason != "scope violation" {
		t.Errorf("reason = %q, want scope violation", ev.Reason)
	}
	if ev.Metadata == nil || ev.Metadata["raw_content_included"] != "false" {
		t.Error("expected metadata with raw_content_included=false")
	}
	if ev.PreviousHash != "" {
		t.Errorf("previousHash = %q, want empty", ev.PreviousHash)
	}
	if ev.EventHash == "" {
		t.Error("expected non-empty eventHash")
	}
}

func TestCreateAuditEventWithPreviousHash(t *testing.T) {
	ev1 := CreateAuditEvent("a", "action-1", "t1", "reason 1", nil, "")
	ev2 := CreateAuditEvent("a", "action-2", "t2", "reason 2", nil, ev1.EventHash)
	if ev2.PreviousHash != ev1.EventHash {
		t.Errorf("previousHash = %q, want %q", ev2.PreviousHash, ev1.EventHash)
	}
}

func TestCreateAuditEventNilMetadata(t *testing.T) {
	ev := CreateAuditEvent("a", "b", "c", "d", nil, "")
	if ev.Metadata == nil {
		t.Error("metadata should not be nil")
	}
}

func TestCanonicalStringDeterministic(t *testing.T) {
	base := AuditEvent{
		ID: "same-id", Timestamp: 1000, Actor: "actor", Action: "action",
		Target: "target", Reason: "reason",
		Metadata:     map[string]string{"z": "last", "a": "first"},
		PreviousHash: "",
	}
	base.EventHash = computeHash(base)
	h1 := base.EventHash

	base2 := AuditEvent{
		ID: "same-id", Timestamp: 1000, Actor: "actor", Action: "action",
		Target: "target", Reason: "reason",
		Metadata:     map[string]string{"a": "first", "z": "last"},
		PreviousHash: "",
	}
	base2.EventHash = computeHash(base2)
	h2 := base2.EventHash

	if h1 != h2 {
		t.Error("canonical string should be deterministic regardless of metadata insertion order")
	}
}

func TestAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	ev1 := CreateAuditEvent("actor-1", "action-1", "target-1", "reason-1", nil, "")
	if err := AppendAuditEvent(path, ev1); err != nil {
		t.Fatalf("AppendAuditEvent: %v", err)
	}
	ev2 := CreateAuditEvent("actor-2", "action-2", "target-2", "reason-2", nil, ev1.EventHash)
	if err := AppendAuditEvent(path, ev2); err != nil {
		t.Fatalf("AppendAuditEvent: %v", err)
	}
	events, err := ReadAuditLog(path)
	if err != nil {
		t.Fatalf("ReadAuditLog: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != ev1.ID {
		t.Error("first event ID mismatch")
	}
	if events[1].ID != ev2.ID {
		t.Error("second event ID mismatch")
	}
	if events[1].PreviousHash != events[0].EventHash {
		t.Error("chain hash mismatch")
	}
}

func TestAppendCreatesDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "auditlog", "events.jsonl")
	ev := CreateAuditEvent("a", "b", "c", "d", nil, "")
	if err := AppendAuditEvent(path, ev); err != nil {
		t.Fatalf("AppendAuditEvent: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func TestReadNonExistent(t *testing.T) {
	events, err := ReadAuditLog("/nonexistent/path.jsonl")
	if err != nil {
		t.Fatalf("ReadAuditLog: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestVerifyValidChain(t *testing.T) {
	events := make([]AuditEvent, 3)
	events[0] = CreateAuditEvent("a", "start", "run-1", "run started", nil, "")
	events[1] = CreateAuditEvent("a", "stop", "run-1", "p45 stop", map[string]string{"raw_content_included": "false"}, events[0].EventHash)
	events[2] = CreateAuditEvent("a", "complete", "run-1", "run complete", nil, events[1].EventHash)
	result := VerifyAuditLog(events)
	if !result.Valid {
		t.Fatalf("expected valid chain, got errors: %v", result.Errors)
	}
}

func TestVerifyDetectsMutation(t *testing.T) {
	ev1 := CreateAuditEvent("a", "start", "run-1", "run started", nil, "")
	ev2 := CreateAuditEvent("a", "stop", "run-1", "p45 stop", nil, ev1.EventHash)
	ev2.Reason = "tampered reason"
	events := []AuditEvent{ev1, ev2}
	result := VerifyAuditLog(events)
	if result.Valid {
		t.Fatal("expected invalid for mutated event")
	}
	matched := false
	for _, e := range result.Errors {
		if strings.Contains(e, "hash mismatch") {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected hash mismatch error, got: %v", result.Errors)
	}
}

func TestVerifyDetectsBrokenChain(t *testing.T) {
	ev1 := CreateAuditEvent("a", "start", "run-1", "run started", nil, "")
	ev2 := CreateAuditEvent("a", "stop", "run-1", "p45 stop", nil, "")
	events := []AuditEvent{ev1, ev2}
	result := VerifyAuditLog(events)
	if result.Valid {
		t.Fatal("expected invalid for broken chain")
	}
	matched := false
	for _, e := range result.Errors {
		if strings.Contains(e, "does not match") {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected chain mismatch error, got: %v", result.Errors)
	}
}

func TestVerifyDetectsReorder(t *testing.T) {
	ev1 := CreateAuditEvent("a", "start", "run-1", "run started", nil, "")
	ev2 := CreateAuditEvent("a", "stop", "run-1", "p45 stop", nil, ev1.EventHash)
	events := []AuditEvent{ev2, ev1}
	result := VerifyAuditLog(events)
	if result.Valid {
		t.Fatal("expected invalid for reordered events")
	}
}

func TestVerifyFirstEventPreviousHash(t *testing.T) {
	ev := CreateAuditEvent("a", "action", "t", "reason", nil, "non-empty")
	events := []AuditEvent{ev}
	result := VerifyAuditLog(events)
	if result.Valid {
		t.Fatal("expected invalid when first event has previousHash")
	}
}

func TestAdapterWritesP45StopEvent(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAdapter(dir, "test-run-001", true)
	if err := adapter.WriteP45StopEvent("task-001", "scope violation: changed file .env matches forbidden path"); err != nil {
		t.Fatalf("WriteP45StopEvent: %v", err)
	}
	events, err := ReadAuditLog(adapter.EventsPath())
	if err != nil {
		t.Fatalf("ReadAuditLog: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Action != "p45_stop_written" {
		t.Errorf("action = %q, want p45_stop_written", ev.Action)
	}
	if ev.Target != "task-001" {
		t.Errorf("target = %q, want task-001", ev.Target)
	}
	if ev.Reason != "scope violation: changed file .env matches forbidden path" {
		t.Errorf("reason = %q, want scope violation message", ev.Reason)
	}
	if ev.Metadata["raw_content_included"] != "false" {
		t.Error("expected raw_content_included=false in metadata")
	}
}

func TestAdapterDisabledNoop(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAdapter(dir, "test-run", false)
	if err := adapter.WriteP45StopEvent("target", "reason"); err != nil {
		t.Fatalf("WriteP45StopEvent: %v", err)
	}
	if _, err := os.Stat(adapter.EventsPath()); !os.IsNotExist(err) {
		t.Error("expected no file created when adapter disabled")
	}
}

func TestAdapterChainContinuity(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAdapter(dir, "chain-test", true)
	if err := adapter.WriteP45StopEvent("task-001", "scope violation 1"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.WriteP45StopEvent("task-001", "scope violation 2"); err != nil {
		t.Fatal(err)
	}
	events, err := ReadAuditLog(adapter.EventsPath())
	if err != nil {
		t.Fatalf("ReadAuditLog: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].PreviousHash != "" {
		t.Errorf("first event previousHash = %q, want empty", events[0].PreviousHash)
	}
	if events[1].PreviousHash != events[0].EventHash {
		t.Errorf("chain broken: second previousHash = %q, want %q", events[1].PreviousHash, events[0].EventHash)
	}
	result := VerifyAuditLog(events)
	if !result.Valid {
		t.Fatalf("chain verification failed: %v", result.Errors)
	}
}

func TestNoRawContentInMetadata(t *testing.T) {
	ev := CreateAuditEvent("c1-forge", "p45_stop_written", "task-001", "scope violation",
		map[string]string{"raw_content_included": "false"}, "")
	if v, ok := ev.Metadata["raw_content_included"]; !ok || v != "false" {
		t.Error("raw_content_included must be false")
	}
	for k := range ev.Metadata {
		if k == "raw_prompt" || k == "raw_diff" || k == "raw_file_content" || k == "secrets" {
			t.Errorf("metadata should not contain raw content key: %s", k)
		}
	}
}

func TestJSONLFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	ev1 := CreateAuditEvent("a", "action-1", "t1", "r1", nil, "")
	ev2 := CreateAuditEvent("a", "action-2", "t2", "r2", nil, ev1.EventHash)
	AppendAuditEvent(path, ev1)
	AppendAuditEvent(path, ev2)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	var parsed AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Errorf("line 1 not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &parsed); err != nil {
		t.Errorf("line 2 not valid JSON: %v", err)
	}
}

func TestEventsPath(t *testing.T) {
	a := NewAdapter("/base/runs/run-test", "test", true)
	expected := "/base/runs/run-test/auditlog/events.jsonl"
	if a.EventsPath() != expected {
		t.Errorf("EventsPath = %q, want %q", a.EventsPath(), expected)
	}
}

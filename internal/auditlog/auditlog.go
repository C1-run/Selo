package auditlog

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AuditEvent struct {
	ID           string            `json:"id"`
	Timestamp    int64             `json:"timestamp"`
	Actor        string            `json:"actor"`
	Action       string            `json:"action"`
	Target       string            `json:"target"`
	Reason       string            `json:"reason"`
	Metadata     map[string]string `json:"metadata"`
	PreviousHash string            `json:"previous_hash"`
	EventHash    string            `json:"event_hash"`
}

type VerificationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func canonicalString(ev AuditEvent) string {
	sortedMeta := make(map[string]string)
	keys := make([]string, 0, len(ev.Metadata))
	for k := range ev.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sortedMeta[k] = ev.Metadata[k]
	}
	metaJSON, _ := json.Marshal(sortedMeta)
	parts := []string{
		ev.ID,
		strconv.FormatInt(ev.Timestamp, 10),
		ev.Actor,
		ev.Action,
		ev.Target,
		ev.Reason,
		string(metaJSON),
		ev.PreviousHash,
	}
	return strings.Join(parts, "|")
}

func computeHash(ev AuditEvent) string {
	h := sha256.Sum256([]byte(canonicalString(ev)))
	return hex.EncodeToString(h[:])
}

func CreateAuditEvent(actor, action, target, reason string, metadata map[string]string, previousHash string) AuditEvent {
	if metadata == nil {
		metadata = map[string]string{}
	}
	ev := AuditEvent{
		ID:           newUUID(),
		Timestamp:    time.Now().UnixMilli(),
		Actor:        actor,
		Action:       action,
		Target:       target,
		Reason:       reason,
		Metadata:     metadata,
		PreviousHash: previousHash,
	}
	ev.EventHash = computeHash(ev)
	return ev
}

func AppendAuditEvent(path string, ev AuditEvent) error {
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func ReadAuditLog(path string) ([]AuditEvent, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	events := make([]AuditEvent, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal([]byte(line), &events[i]); err != nil {
			return nil, fmt.Errorf("line %d: %w", i, err)
		}
	}
	return events, nil
}

func VerifyAuditLog(events []AuditEvent) VerificationResult {
	var errs []string
	for i, ev := range events {
		storedHash := ev.EventHash
		ev.EventHash = ""
		expected := computeHash(ev)
		if storedHash != expected {
			errs = append(errs, fmt.Sprintf("event[%d]: hash mismatch — content may have been mutated", i))
			continue
		}
		if i == 0 {
			if ev.PreviousHash != "" {
				errs = append(errs, fmt.Sprintf("event[0]: expected previousHash to be empty, got %q", ev.PreviousHash))
			}
		} else {
			prev := events[i-1]
			if ev.PreviousHash != prev.EventHash {
				errs = append(errs, fmt.Sprintf("event[%d]: previousHash %q does not match event[%d].eventHash %q", i, ev.PreviousHash, i-1, prev.EventHash))
			}
		}
	}
	return VerificationResult{Valid: len(errs) == 0, Errors: errs}
}

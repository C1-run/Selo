package auditlog

import (
	"path/filepath"
)

type Adapter struct {
	Enabled     bool
	RunsDir     string
	RunID       string
	lastHash    string
}

func NewAdapter(runsDir, runID string, enabled bool) *Adapter {
	return &Adapter{
		Enabled: enabled,
		RunsDir: runsDir,
		RunID:   runID,
	}
}

func (a *Adapter) EventsPath() string {
	return filepath.Join(a.RunsDir, "auditlog", "events.jsonl")
}

func (a *Adapter) WriteP45StopEvent(target, reason string) error {
	if !a.Enabled {
		return nil
	}
	ev := CreateAuditEvent("c1-forge", "p45_stop_written", target, reason,
		map[string]string{"raw_content_included": "false"},
		a.lastHash,
	)
	if err := AppendAuditEvent(a.EventsPath(), ev); err != nil {
		return err
	}
	a.lastHash = ev.EventHash
	return nil
}

package mcpserver

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runRequests(t *testing.T, lines []string) []map[string]any {
	t.Helper()
	in := strings.Join(lines, "\n") + "\n"
	var out bytes.Buffer
	srv := NewWithTools(strings.NewReader(in), &out, defaultTools())
	if err := srv.Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("response line is not JSON: %v\n%s", err, line)
		}
		responses = append(responses, m)
	}
	return responses
}

func TestMCPServerInitializeAndListTools(t *testing.T) {
	resp := runRequests(t, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	})
	if len(resp) != 2 {
		t.Fatalf("got %d responses (notification must be silent), want 2", len(resp))
	}
	init := resp[0]["result"].(map[string]any)
	if init["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v", init["protocolVersion"])
	}
	tools := resp[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Errorf("tools/list returned %d tools, want 3", len(tools))
	}
}

func TestMCPServerCheckDiffTool(t *testing.T) {
	dir := t.TempDir()
	// The worktree file contains a claim and a secret; the diff touches it.
	content := "token = \"supersecretvalue1\"\n// production_ready everywhere\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	diff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n+token = \"supersecretvalue1\"\n"

	params, _ := json.Marshal(map[string]any{
		"name": "selo_check_diff",
		"arguments": map[string]any{
			"diff":             diff,
			"worktree_path":    dir,
			"forbidden_claims": []string{"PRODUCTION_READY"},
		},
	})
	req := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":` + string(params) + `}`
	resp := runRequests(t, []string{req})
	if len(resp) != 1 {
		t.Fatalf("got %d responses, want 1", len(resp))
	}
	result := resp[0]["result"].(map[string]any)
	content0 := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content0, "HIT (secret)") {
		t.Errorf("expected a secret hit, got: %s", content0)
	}
	if !strings.Contains(content0, "HIT (forbidden claim)") {
		t.Errorf("expected a forbidden claim hit, got: %s", content0)
	}
}

func TestMCPServerUnknownToolAndMethod(t *testing.T) {
	resp := runRequests(t, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"bogus/method"}`,
	})
	if len(resp) != 2 {
		t.Fatalf("got %d responses, want 2", len(resp))
	}
	if resp[0]["error"] == nil {
		t.Error("unknown tool should return a JSON-RPC error")
	}
	if resp[1]["error"] == nil {
		t.Error("unknown method should return a JSON-RPC error")
	}
}

func TestMCPPreflightTool(t *testing.T) {
	params, _ := json.Marshal(map[string]any{
		"name":      "selo_preflight",
		"arguments": map[string]any{"files_changed": 50, "patch_lines": 10},
	})
	resp := runRequests(t, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + string(params) + `}`,
	})
	text := resp[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "EXCEEDS LIMITS") {
		t.Errorf("expected limit violation, got: %s", text)
	}
}

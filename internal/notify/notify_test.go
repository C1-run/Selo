package notify

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStdoutNotifier(t *testing.T) {
	var buf bytes.Buffer
	n := &StdoutNotifier{Writer: &buf}
	err := n.Notify(context.Background(), Notification{
		TaskID:       "test-1",
		FinalVerdict: "SUCCESS_WITH_RECEIPT",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "test-1") {
		t.Errorf("output missing task ID: %s", output)
	}
	if !strings.Contains(output, "SUCCESS_WITH_RECEIPT") {
		t.Errorf("output missing verdict: %s", output)
	}
}

func TestStdoutNotifierNoop(t *testing.T) {
	var buf bytes.Buffer
	n := &StdoutNotifier{Writer: &buf}
	err := n.Notify(context.Background(), Notification{
		TaskID:       "test-2",
		FinalVerdict: "NOOP_WITH_RECEIPT",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "NOOP_WITH_RECEIPT") {
		t.Errorf("output missing verdict: %s", output)
	}
}

func TestNtfyNotifierPostsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Title") == "" {
			t.Error("missing Title header")
		}
		if r.Header.Get("Priority") == "" {
			t.Error("missing Priority header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNtfyNotifier(server.URL, 0)
	n.Client = server.Client()
	err := n.Notify(context.Background(), Notification{
		TaskID:       "ntfy-test",
		FinalVerdict: "SUCCESS_WITH_RECEIPT",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestNtfyNotifier5xxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	n := NewNtfyNotifier(server.URL, 0)
	n.Client = server.Client()
	err := n.Notify(context.Background(), Notification{
		TaskID:       "ntfy-err",
		FinalVerdict: "SUCCESS_WITH_RECEIPT",
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention HTTP status: %v", err)
	}
}

func TestNotificationFailureDoesNotChangeVerdict(t *testing.T) {
	// The notifier receives a verdict and sends it — failure does not alter the verdict.
	// This tests that the notifier itself is stateless and does not modify the notification.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	n := NewNtfyNotifier(server.URL, 0)
	n.Client = server.Client()
	notif := Notification{
		TaskID:       "fail-test",
		FinalVerdict: "FAILED_SAFETY",
	}
	err := n.Notify(context.Background(), notif)
	if err == nil {
		t.Fatal("expected error")
	}
	if notif.FinalVerdict != "FAILED_SAFETY" {
		t.Errorf("verdict changed after notification failure: %s", notif.FinalVerdict)
	}
}

func TestSafetyFailureUsesHighPriority(t *testing.T) {
	var capturedPriority string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPriority = r.Header.Get("Priority")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNtfyNotifier(server.URL, 0)
	n.Client = server.Client()
	err := n.Notify(context.Background(), Notification{
		TaskID:        "safety-test",
		FinalVerdict:  "FAILED_SAFETY",
		SafetyFailure: true,
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if capturedPriority != "5" {
		t.Errorf("Priority = %s, want 5 for safety failure", capturedPriority)
	}
}

func TestNeedsHumanUsesWarningPriority(t *testing.T) {
	var capturedPriority string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPriority = r.Header.Get("Priority")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNtfyNotifier(server.URL, 0)
	n.Client = server.Client()
	err := n.Notify(context.Background(), Notification{
		TaskID:      "human-test",
		NeedsHuman:  true,
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if capturedPriority != "4" {
		t.Errorf("Priority = %s, want 4 for needs_human", capturedPriority)
	}
}

func TestDisabledNotifierNoops(t *testing.T) {
	n := NewDisabledNotifier()
	err := n.Notify(context.Background(), Notification{
		TaskID:       "disabled-test",
		FinalVerdict: "SUCCESS_WITH_RECEIPT",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestNtfyNotifierPostsBody(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		capturedBody = buf.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNtfyNotifier(server.URL, 0)
	n.Client = server.Client()
	err := n.Notify(context.Background(), Notification{
		TaskID:         "body-test",
		Repo:           "/tmp/repo",
		FinalVerdict:   "SUCCESS_WITH_RECEIPT",
		TestsPassed:    true,
		ScansPassed:    true,
		FilesChanged:   3,
		ReceiptPath:    "/tmp/receipt.json",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if !strings.Contains(capturedBody, "body-test") {
		t.Errorf("body missing task ID: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "SUCCESS_WITH_RECEIPT") {
		t.Errorf("body missing verdict: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "/tmp/repo") {
		t.Errorf("body missing repo: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "/tmp/receipt.json") {
		t.Errorf("body missing receipt path: %s", capturedBody)
	}
}

func TestNewNotifierFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		wantNil  bool
		wantWarn bool
	}{
		{"stdout default", Config{Mode: "stdout"}, false, false},
		{"disabled", Config{Mode: "disabled"}, false, false},
		{"ntfy with url", Config{Mode: "ntfy", NtfyURL: "https://ntfy.sh/test", TimeoutSeconds: 5}, false, false},
		{"ntfy without url", Config{Mode: "ntfy"}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, warn := NewNotifierFromConfig(tt.cfg)
			if n == nil {
				t.Error("expected non-nil notifier")
			}
			if tt.wantWarn && warn == "" {
				t.Error("expected warning but got empty")
			}
			if !tt.wantWarn && warn != "" {
				t.Errorf("unexpected warning: %s", warn)
			}
		})
	}
}

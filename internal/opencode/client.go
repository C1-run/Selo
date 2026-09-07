package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with a running OpenCode serve instance via REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new OpenCode REST API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// CreateSession creates a new OpenCode session.
func (c *Client) CreateSession(ctx context.Context) (*Session, error) {
	resp, err := c.do(ctx, http.MethodPost, "/session", nil)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer resp.Body.Close()

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	return &session, nil
}

// Prompt sends a message to a session and returns the response.
func (c *Client) Prompt(ctx context.Context, sessionID, prompt string) (*PromptResponse, error) {
	body, _ := json.Marshal(PromptRequest{Prompt: prompt})
	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/session/%s/prompt", sessionID), body)
	if err != nil {
		return nil, fmt.Errorf("prompt session: %w", err)
	}
	defer resp.Body.Close()

	var pr PromptResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode prompt response: %w", err)
	}
	return &pr, nil
}

// GetMessages returns all messages in a session.
func (c *Client) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/session/%s/messages", sessionID), nil)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}
	defer resp.Body.Close()

	var messages []Message
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("decode messages: %w", err)
	}
	return messages, nil
}

// Abort stops a running prompt in a session.
func (c *Client) Abort(ctx context.Context, sessionID string) error {
	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/session/%s/abort", sessionID), nil)
	if err != nil {
		return fmt.Errorf("abort session: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// Close cleans up the client.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// do performs an HTTP request and returns the response.
func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	return resp, nil
}

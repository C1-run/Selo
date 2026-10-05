// Package mcpserver exposes Selo's safety checks as a Model Context Protocol
// server over stdio. It is the in-CLI replacement for the retired Selo Lite
// project: OpenCode and other MCP clients talk to the real checks instead of
// a disconnected reimplementation.
package mcpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const protocolVersion = "2024-11-05"
const serverVersion = "0.3.0"

// Tool is a single MCP-exposed capability.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Call        func(args map[string]any) (string, error)
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server serves the tools over newline-delimited JSON-RPC 2.0 on the given
// reader/writer (stdio in production).
type Server struct {
	tools []Tool
	in    io.Reader
	out   io.Writer
}

// New returns a stdio MCP server exposing the default tool set.
func New(in io.Reader, out io.Writer) *Server {
	return &Server{tools: defaultTools(), in: in, out: out}
}

// NewWithTools returns a server with an explicit tool set (used by tests).
func NewWithTools(in io.Reader, out io.Writer, tools []Tool) *Server {
	return &Server{tools: tools, in: in, out: out}
}

// Serve reads requests until EOF. Notifications produce no response; every
// request gets exactly one JSON-RPC response line.
func (s *Server) Serve() error {
	dec := json.NewDecoder(bufio.NewReader(s.in))
	for {
		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode request: %w", err)
		}
		resp := s.handle(&req)
		if resp == nil {
			continue // notification
		}
		line, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
		if _, err := fmt.Fprintf(s.out, "%s\n", line); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
}

func (s *Server) handle(req *rpcRequest) *rpcResponse {
	respond := func(result any, rerr *rpcError) *rpcResponse {
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr}
	}

	switch req.Method {
	case "initialize":
		return respond(map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "selo", "version": serverVersion},
		}, nil)
	case "notifications/initialized", "notifications/cancelled":
		return nil
	case "ping":
		return respond(map[string]any{}, nil)
	case "tools/list":
		tools := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.Schema,
			})
		}
		return respond(map[string]any{"tools": tools}, nil)
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return respond(nil, &rpcError{Code: -32602, Message: "invalid params"})
			}
		}
		for _, t := range s.tools {
			if t.Name != params.Name {
				continue
			}
			text, err := t.Call(params.Arguments)
			if err != nil {
				return respond(map[string]any{
					"content": []map[string]any{{"type": "text", "text": err.Error()}},
					"isError": true,
				}, nil)
			}
			return respond(map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
			}, nil)
		}
		return respond(nil, &rpcError{Code: -32602, Message: fmt.Sprintf("unknown tool %q", params.Name)})
	default:
		if req.ID == nil {
			return nil // unknown notification: ignore
		}
		return respond(nil, &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)})
	}
}

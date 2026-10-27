// Package mcpserver exposes Slot to MCP clients (Claude Desktop, Cursor, …)
// over stdio. It is a thin adapter: every tool calls the admin API through
// slotcli, so the MCP server never touches the database.
package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/puppe1990/slot/internal/slotcli"
)

const protocolVersion = "2024-11-05"

type Server struct {
	API     *slotcli.Client
	Version string
}

func New(api *slotcli.Client, version string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{API: api, Version: version}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Run reads newline-delimited JSON-RPC from in and writes responses to out.
// Notifications (no id) never get a reply, per JSON-RPC.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var call request
		if err := json.Unmarshal(line, &call); err != nil {
			_ = encoder.Encode(response{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "parse error: " + err.Error()},
			})
			continue
		}
		reply, ok := s.handle(ctx, call)
		if !ok {
			continue
		}
		if err := encoder.Encode(reply); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, call request) (response, bool) {
	reply := response{JSONRPC: "2.0", ID: call.ID}
	notification := len(call.ID) == 0

	switch call.Method {
	case "initialize":
		reply.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "slot", "version": s.Version},
		}
	case "notifications/initialized":
		return response{}, false
	case "ping":
		reply.Result = map[string]any{}
	case "tools/list":
		reply.Result = map[string]any{"tools": tools()}
	case "tools/call":
		result, err := s.callTool(ctx, call.Params)
		if err != nil {
			reply.Result = textResult(err.Error(), true)
		} else {
			reply.Result = textResult(result, false)
		}
	default:
		if notification {
			return response{}, false
		}
		reply.Error = &rpcError{Code: -32601, Message: "method not found: " + call.Method}
	}

	if notification {
		return response{}, false
	}
	return reply, true
}

func textResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isError,
	}
}

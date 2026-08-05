package mcp

import (
	"bufio"
	"context"
	"dossier/internal/core"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// JSONRPCRequest represents a generic JSON-RPC 2.0 request frame.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id,omitempty"`
}

// JSONRPCResponse represents a generic JSON-RPC 2.0 response frame.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      any             `json:"id,omitempty"`
}

// JSONRPCError represents a generic JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Server implements the MCP server protocol over stdio.
type Server struct {
	svc      *core.Service
	reader   io.Reader
	writer   io.Writer
	syncChan chan struct{}
	doneChan chan struct{}
}

// NewServer creates a new Server instance.
func NewServer(svc *core.Service, r io.Reader, w io.Writer) *Server {
	return &Server{
		svc:      svc,
		reader:   r,
		writer:   w,
		syncChan: make(chan struct{}, 1),
		doneChan: make(chan struct{}),
	}
}

func (s *Server) triggerSync() {
	select {
	case s.syncChan <- struct{}{}:
	default:
	}
}

func (s *Server) syncDebouncer(ctx context.Context) {
	defer close(s.doneChan)

	debounceInterval := 2 * time.Second
	var pending bool

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-s.syncChan:
			if !ok {
				if pending {
					syncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, _ = s.svc.Sync(syncCtx)
					cancel()
				}
				return
			}
			pending = true
		}

		timer := time.NewTimer(debounceInterval)
	settle:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case _, ok := <-s.syncChan:
				if !ok {
					timer.Stop()
					syncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, _ = s.svc.Sync(syncCtx)
					cancel()
					return
				}
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(debounceInterval)
			case <-timer.C:
				syncCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, _ = s.svc.Sync(syncCtx)
				cancel()
				pending = false
				break settle
			}
		}
	}
}

// Run starts the stdio read loop.
func (s *Server) Run(ctx context.Context) error {
	go s.syncDebouncer(ctx)

	scanner := bufio.NewScanner(s.reader)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error", nil)
			continue
		}

		s.handleRequest(ctx, req)
	}

	err := scanner.Err()

	close(s.syncChan)
	select {
	case <-s.doneChan:
	case <-time.After(5 * time.Second):
	}

	return err
}

func (s *Server) handleRequest(ctx context.Context, req JSONRPCRequest) {
	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		type initResult struct {
			ProtocolVersion string `json:"protocolVersion"`
			Capabilities    struct {
				Tools struct{} `json:"tools"`
			} `json:"capabilities"`
			ServerInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		}

		var res initResult
		res.ProtocolVersion = "2024-11-05"
		res.ServerInfo.Name = "dossier"
		res.ServerInfo.Version = "1.0.0"

		s.sendResult(req.ID, res)

	case "notifications/initialized":
		// Do nothing, initialized notification

	case "ping":
		s.sendResult(req.ID, map[string]any{})

	case "tools/list":
		tools := getToolDefinitions()
		s.sendResult(req.ID, map[string]any{"tools": tools})

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params", nil)
			return
		}

		s.handleToolCall(ctx, req.ID, params.Name, params.Arguments)

	default:
		if !isNotification {
			s.sendError(req.ID, -32601, fmt.Sprintf("Method %s not found", req.Method), nil)
		}
	}
}

func (s *Server) sendResult(id any, result any) {
	resBytes, err := json.Marshal(result)
	if err != nil {
		s.sendError(id, -32603, "Internal marshal error", nil)
		return
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resBytes,
		ID:      id,
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return
	}

	respBytes = append(respBytes, '\n')
	_, _ = s.writer.Write(respBytes)
}

func (s *Server) sendError(id any, code int, message string, data any) {
	var dataBytes json.RawMessage
	if data != nil {
		dataBytes, _ = json.Marshal(data)
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    dataBytes,
		},
		ID: id,
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return
	}

	respBytes = append(respBytes, '\n')
	_, _ = s.writer.Write(respBytes)
}

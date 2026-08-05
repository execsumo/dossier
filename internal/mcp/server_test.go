package mcp

import (
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type mockHarnessRegistry struct{}

func (m *mockHarnessRegistry) All() []core.Harness { return nil }
func (m *mockHarnessRegistry) Get(name string) (core.Harness, error) {
	return nil, nil
}

type mockClock struct{}

func (m *mockClock) Now() time.Time { return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC) }

type mockTokenizer struct{}

func (m *mockTokenizer) Estimate(text string) int { return len(text) / 4 }

type mockSearcher struct{}

func (m *mockSearcher) Search(ctx context.Context, query string, scope core.SearchScope) ([]core.Hit, error) {
	return []core.Hit{
		{DossierID: "dos_1", DossierName: "Test Dossier", Snippet: "matching search text"},
	}, nil
}

func TestMCPServer(t *testing.T) {
	// Setup core service with fake store
	fakeStore := store.NewFakeStore()
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{}
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	cfg := core.Config{TokenTarget: 100}

	svc := core.NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)

	// Pre-populate a dossier
	d := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_1",
			Name:          "Test Dossier",
			Slug:          "test-dossier",
			Status:        core.StatusActive,
			Importance:    core.ImportanceHigh,
			Urgency:       core.UrgencyHigh,
			CreatedAt:     clk.Now(),
			UpdatedAt:     clk.Now(),
			LastTouchedAt: clk.Now(),
		},
		DistilledState: core.DistilledState{
			Body: "# Test Dossier Distilled State",
		},
	}
	fakeStore.Dossiers["dos_1"] = d
	fakeStore.Revisions["dos_1"] = "rev_1"

	// JSON-RPC message sequence for initialize, tools/list, and tools/call (recall)
	inputSequence := []string{
		`{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05"},"id":1}`,
		`{"jsonrpc":"2.0","method":"tools/list","id":2}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_recall","arguments":{"id":"dos_1"}},"id":3}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_search","arguments":{"query":"matching"}},"id":4}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_recall","arguments":{"id":"dos_nonexistent"}},"id":5}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_promote","arguments":{"name":"New MCP Dossier","distilled_state_markdown":"# New","session_content":"hello","force":true}},"id":6}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_link","arguments":{"id":"dos_1","session_content":"linked content"}},"id":7}`,
	}

	inBuf := bytes.NewBufferString(strings.Join(inputSequence, "\n") + "\n")
	var outBuf bytes.Buffer

	server := NewServer(svc, inBuf, &outBuf)
	ctx := context.Background()

	err := server.Run(ctx)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("server.Run failed: %v", err)
	}

	// Parse responses line-by-line
	outputLines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	if len(outputLines) != 7 {
		t.Fatalf("expected 7 response lines, got %d:\n%s", len(outputLines), outBuf.String())
	}

	// 1. Assert initialize response
	var resp1 JSONRPCResponse
	if err := json.Unmarshal([]byte(outputLines[0]), &resp1); err != nil {
		t.Fatalf("unmarshal resp1 failed: %v", err)
	}
	if resp1.ID.(float64) != 1 {
		t.Errorf("expected ID 1, got %v", resp1.ID)
	}
	var initRes map[string]any
	_ = json.Unmarshal(resp1.Result, &initRes)
	if initRes["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected version 2024-11-05, got %v", initRes["protocolVersion"])
	}

	// 2. Assert tools/list response
	var resp2 JSONRPCResponse
	if err := json.Unmarshal([]byte(outputLines[1]), &resp2); err != nil {
		t.Fatalf("unmarshal resp2 failed: %v", err)
	}
	if resp2.ID.(float64) != 2 {
		t.Errorf("expected ID 2, got %v", resp2.ID)
	}
	var listRes map[string]any
	_ = json.Unmarshal(resp2.Result, &listRes)
	tools, ok := listRes["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Errorf("expected non-empty tools list, got %+v", listRes)
	}

	// 3. Assert tools/call (recall) response
	var resp3 JSONRPCResponse
	if err := json.Unmarshal([]byte(outputLines[2]), &resp3); err != nil {
		t.Fatalf("unmarshal resp3 failed: %v", err)
	}
	if resp3.ID.(float64) != 3 {
		t.Errorf("expected ID 3, got %v", resp3.ID)
	}
	var callRes map[string]any
	_ = json.Unmarshal(resp3.Result, &callRes)
	contentList, ok := callRes["content"].([]any)
	if !ok || len(contentList) != 1 {
		t.Fatalf("expected content list of size 1, got %+v", callRes)
	}
	textItem := contentList[0].(map[string]any)
	if textItem["type"] != "text" {
		t.Errorf("expected type text, got %v", textItem["type"])
	}

	// Unmarshal envelope
	var env mcpEnvelope
	if err := json.Unmarshal([]byte(textItem["text"].(string)), &env); err != nil {
		t.Fatalf("failed to unmarshal env: %v", err)
	}
	if !env.OK {
		t.Errorf("expected recall ok, got false")
	}
	recallResultMap := env.Data.(map[string]any)
	if recallResultMap["distilled_state"] != "# Test Dossier Distilled State" {
		t.Errorf("expected distilled state, got %v", recallResultMap["distilled_state"])
	}

	// 4. Assert tools/call (search) response
	var resp4 JSONRPCResponse
	_ = json.Unmarshal([]byte(outputLines[3]), &resp4)
	var callRes4 map[string]any
	_ = json.Unmarshal(resp4.Result, &callRes4)
	contentList4 := callRes4["content"].([]any)
	textItem4 := contentList4[0].(map[string]any)
	var env4 mcpEnvelope
	_ = json.Unmarshal([]byte(textItem4["text"].(string)), &env4)
	if !env4.OK {
		t.Errorf("expected search ok")
	}
	hitsList := env4.Data.([]any)
	if len(hitsList) != 1 {
		t.Errorf("expected 1 search hit, got %d", len(hitsList))
	}

	// 5. Assert tools/call (error mapping) response for nonexistent dossier recall
	var resp5 JSONRPCResponse
	_ = json.Unmarshal([]byte(outputLines[4]), &resp5)
	var callRes5 map[string]any
	_ = json.Unmarshal(resp5.Result, &callRes5)
	contentList5 := callRes5["content"].([]any)
	textItem5 := contentList5[0].(map[string]any)
	var env5 mcpEnvelope
	_ = json.Unmarshal([]byte(textItem5["text"].(string)), &env5)
	if env5.OK {
		t.Errorf("expected nonexistent recall to fail (OK: false)")
	}
	if env5.Error == nil {
		t.Fatalf("expected error object, got nil")
	}
	if env5.Error.Code != ErrCodeNotFound {
		t.Errorf("expected error code %s, got %s", ErrCodeNotFound, env5.Error.Code)
	}

	// 6. Assert tools/call (promote) response
	var resp6 JSONRPCResponse
	_ = json.Unmarshal([]byte(outputLines[5]), &resp6)
	var callRes6 map[string]any
	_ = json.Unmarshal(resp6.Result, &callRes6)
	contentList6 := callRes6["content"].([]any)
	textItem6 := contentList6[0].(map[string]any)
	var env6 mcpEnvelope
	_ = json.Unmarshal([]byte(textItem6["text"].(string)), &env6)
	if !env6.OK {
		t.Errorf("expected promote ok, got error: %+v", env6.Error)
	}

	// 7. Assert tools/call (link) response
	var resp7 JSONRPCResponse
	_ = json.Unmarshal([]byte(outputLines[6]), &resp7)
	var callRes7 map[string]any
	_ = json.Unmarshal(resp7.Result, &callRes7)
	contentList7 := callRes7["content"].([]any)
	textItem7 := contentList7[0].(map[string]any)
	var env7 mcpEnvelope
	_ = json.Unmarshal([]byte(textItem7["text"].(string)), &env7)
	if !env7.OK {
		t.Errorf("expected link ok, got error: %+v", env7.Error)
	}
}

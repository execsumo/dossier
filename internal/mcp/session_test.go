package mcp

import (
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// newSessionTestService builds a core.Service backed by a fake store holding one dossier.
func newSessionTestService(t *testing.T) *core.Service {
	t.Helper()
	fakeStore := store.NewFakeStore()
	svc := core.NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{TokenTarget: 100}, nil)

	clk := &mockClock{}
	fakeStore.Dossiers["dos_1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_1", Name: "Test", Slug: "test-dossier", Status: core.StatusActive,
			CreatedAt: clk.Now(), UpdatedAt: clk.Now(), LastTouchedAt: clk.Now(),
		},
		DistilledState: core.DistilledState{Body: "# Test"},
	}
	fakeStore.Revisions["dos_1"] = "rev_1"
	return svc
}

// callTool drives a single tools/call request through the server and returns the envelope.
func callTool(t *testing.T, svc *core.Service, name, args string) mcpEnvelope {
	t.Helper()
	in := bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":%q,"arguments":%s},"id":1}`+"\n", name, args))
	var out bytes.Buffer
	if err := NewServer(svc, in, &out).Run(context.Background()); err != nil && err.Error() != "EOF" {
		t.Fatalf("server.Run: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (out=%s)", err, out.String())
	}
	var callRes map[string]any
	if err := json.Unmarshal(resp.Result, &callRes); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	content := callRes["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var env mcpEnvelope
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return env
}

// TestMCPUpdateLeadAndStatus proves the consolidated dossier_update tool routes lead and
// status through the unified Save path and they round-trip back via dossier_recall.
func TestMCPUpdateLeadAndStatus(t *testing.T) {
	svc := newSessionTestService(t)

	upd := callTool(t, svc, "dossier_update", `{"id":"dos_1","lead":"Alice","status":"waiting"}`)
	if !upd.OK {
		t.Fatalf("expected update ok, got error: %+v", upd.Error)
	}

	rec := callTool(t, svc, "dossier_recall", `{"id":"dos_1"}`)
	if !rec.OK {
		t.Fatalf("expected recall ok, got error: %+v", rec.Error)
	}
	raw, _ := json.Marshal(rec.Data)
	if !strings.Contains(string(raw), "Alice") {
		t.Errorf("expected recalled dossier to carry lead Alice, got %s", raw)
	}
	if !strings.Contains(string(raw), "waiting") {
		t.Errorf("expected recalled dossier status waiting, got %s", raw)
	}
}

// TestMCPSwitchResolvesSessionFromEnv proves an agent can switch/read the active dossier
// without supplying session_id: the MCP server resolves it from CLAUDE_CODE_SESSION_ID,
// and the binding round-trips through dossier_session under the same session.
func TestMCPSwitchResolvesSessionFromEnv(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "sess-mcp-A")

	svc := newSessionTestService(t)

	// switch/bind with id only — no session_id param.
	sw := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if !sw.OK {
		t.Fatalf("expected switch/bind ok, got error: %+v", sw.Error)
	}

	// active session binding with no args — must report the binding made above for sess-mcp-A.
	act := callTool(t, svc, "dossier_session", `{}`)
	if !act.OK {
		t.Fatalf("expected active ok, got error: %+v", act.Error)
	}
	raw, _ := json.Marshal(act.Data)
	if !strings.Contains(string(raw), "dos_1") {
		t.Errorf("expected active binding to reference dos_1, got %s", raw)
	}
}

// TestMCPSwitchNoSessionDegradesVisibly proves the MCP path errors visibly (rather than
// silently binding the shared sess_default bucket) when no session id is resolvable.
func TestMCPSwitchNoSessionDegradesVisibly(t *testing.T) {
	t.Setenv("DOSSIER_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")

	svc := newSessionTestService(t)

	env := callTool(t, svc, "dossier_session", `{"id":"dos_1"}`)
	if env.OK {
		t.Fatalf("expected switch/bind to fail without a session id")
	}
	if env.Error == nil || env.Error.Code != ErrCodeHarnessCapUnavailable {
		t.Errorf("expected error code %s, got %+v", ErrCodeHarnessCapUnavailable, env.Error)
	}

	// And it must not have silently bound the shared bucket.
	if b, _ := svc.Active(context.Background(), core.ActiveReq{SessionID: "sess_default"}); b.Data != nil {
		t.Errorf("expected no sess_default binding, got %+v", b.Data)
	}
}

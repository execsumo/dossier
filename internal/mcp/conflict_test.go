package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestMCPConflictToolsUseStandardEnvelope(t *testing.T) {
	fake := store.NewFakeStore()
	fake.Dossiers["dos_mcp"] = &core.Dossier{
		Frontmatter:    core.Frontmatter{ID: "dos_mcp", Name: "MCP", Slug: "mcp", Status: core.StatusSpark, Priority: core.PriorityMedium},
		DistilledState: core.DistilledState{Body: "shared"},
	}
	fake.Revisions["dos_mcp"] = "rev1"
	fake.Conflicts["conf_mcp"] = &core.Conflict{ID: "conf_mcp", DossierID: "dos_mcp", RejectedBody: "mine"}
	svc := core.NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{}, nil)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_conflicts","arguments":{}},"id":2}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_conflicts","arguments":{"conflict_id":"conf_mcp"}},"id":3}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_resolve_conflict","arguments":{"conflict_id":"conf_mcp","choice":"bad"}},"id":4}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_resolve_conflict","arguments":{"conflict_id":"missing","choice":"keep_shared"}},"id":5}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_resolve_conflict","arguments":{"conflict_id":"conf_mcp","choice":"keep_shared"}},"id":6}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	server := NewServer(svc, strings.NewReader(input), &output)
	if err := server.Run(context.Background()); err != nil && err.Error() != "EOF" {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("responses = %d, want 6", len(lines))
	}
	for i, want := range []bool{true, true, false, false, true} {
		var response JSONRPCResponse
		if err := json.Unmarshal([]byte(lines[i+1]), &response); err != nil {
			t.Fatal(err)
		}
		var call map[string]any
		if err := json.Unmarshal(response.Result, &call); err != nil {
			t.Fatal(err)
		}
		content := call["content"].([]any)[0].(map[string]any)
		var envelope mcpEnvelope
		if err := json.Unmarshal([]byte(content["text"].(string)), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.OK != want {
			t.Fatalf("response %d ok = %v, want %v: %+v", i+1, envelope.OK, want, envelope.Error)
		}
		if i == 1 {
			if !strings.Contains(content["text"].(string), `"shared":"shared"`) || !strings.Contains(content["text"].(string), `"mine":"mine"`) {
				t.Fatalf("detail missing shared/mine: %s", content["text"])
			}
		}
		if i == 2 && envelope.Error.Code != ErrCodeInvalidFrontmatter {
			t.Fatalf("invalid choice code = %q", envelope.Error.Code)
		}
		if i == 3 && envelope.Error.Code != ErrCodeNotFound {
			t.Fatalf("unknown conflict code = %q", envelope.Error.Code)
		}
	}
}

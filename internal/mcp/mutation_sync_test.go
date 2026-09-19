package mcp

import (
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mutationService(t *testing.T, syncer core.Syncer) (*core.Service, *store.FakeStore) {
	t.Helper()
	fs := store.NewFakeStore()
	fs.Dossiers["dos_1"] = &core.Dossier{
		Frontmatter:    core.Frontmatter{ID: "dos_1", Name: "Test Dossier", Slug: "test-dossier", Status: core.StatusActive, Priority: core.PriorityMedium},
		DistilledState: core.DistilledState{Body: "old"},
	}
	fs.Revisions["dos_1"] = "rev_1"
	return core.NewService(fs, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{}, syncer), fs
}

func mutationCall(t *testing.T, svc *core.Service, name, args string) (mcpEnvelope, int) {
	t.Helper()
	out := &safeWriter{}
	server := NewServer(svc, bytes.NewBuffer(nil), out)
	server.handleRequest(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  []byte(fmt.Sprintf(`{"name":%q,"arguments":%s}`, name, args)),
		ID:      1,
	})
	var response JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &response); err != nil {
		t.Fatalf("unmarshal response for %s: %v (%s)", name, err, out.String())
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("unmarshal tool result for %s: %v", name, err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("tool %s returned %d content items", name, len(result.Content))
	}
	var env mcpEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &env); err != nil {
		t.Fatalf("unmarshal envelope for %s: %v", name, err)
	}
	return env, len(server.syncChan)
}

func TestMCPMutationsEnqueueBackgroundSync(t *testing.T) {
	tests := []struct {
		name  string
		args  string
		setup func(*store.FakeStore)
	}{
		{"dossier_update", `{"id":"dos_1","lead":"psmith"}`, nil},
		{"dossier_save", `{"id":"dos_1","distilled_state_markdown":"# New"}`, nil},
		{"dossier_promote", `{"name":"New Thread","force":true}`, nil},
		{"dossier_link", `{"id":"dos_1","session_content":"linked"}`, nil},
		{"dossier_merge", `{"source_id":"dos_2","target_id":"dos_1"}`, func(fs *store.FakeStore) {
			fs.Dossiers["dos_2"] = &core.Dossier{
				Frontmatter:    core.Frontmatter{ID: "dos_2", Name: "Source", Slug: "source", Status: core.StatusActive, Priority: core.PriorityMedium},
				DistilledState: core.DistilledState{Body: "source"},
			}
			fs.Revisions["dos_2"] = "rev_2"
		}},
		{"dossier_rename", `{"id":"dos_1","new_name":"Renamed","base_revision":"rev_1"}`, nil},
		{"dossier_resolve_conflict", `{"conflict_id":"conf_1","choice":"keep_shared"}`, func(fs *store.FakeStore) {
			fs.Conflicts["conf_1"] = &core.Conflict{ID: "conf_1", DossierID: "dos_1"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, fs := mutationService(t, &blockingSyncer{})
			if tt.setup != nil {
				tt.setup(fs)
			}
			env, queued := mutationCall(t, svc, tt.name, tt.args)
			if !env.OK {
				t.Fatalf("%s failed: %+v", tt.name, env.Error)
			}
			if queued != 1 {
				t.Fatalf("%s queued %d syncs, want 1", tt.name, queued)
			}
			if tt.name == "dossier_update" && fs.Dossiers["dos_1"].Frontmatter.Lead != "psmith" {
				t.Fatalf("update did not persist lead: %+v", fs.Dossiers["dos_1"].Frontmatter)
			}
		})
	}
}

func TestMCPFailedMutationsDoNotEnqueueBackgroundSync(t *testing.T) {
	tests := []struct {
		name  string
		args  string
		setup func(*store.FakeStore)
	}{
		{"dossier_update", `{"id":"missing","lead":"psmith"}`, nil},
		{"dossier_save", `{"id":"dos_1","base_revision":"stale","distilled_state_markdown":"# New"}`, nil},
		{"dossier_promote", `{"name":"Test Dossier"}`, nil},
		{"dossier_link", `{"session_content":"linked"}`, nil},
		{"dossier_merge", `{"source_id":"dos_2","target_id":"dos_1"}`, func(fs *store.FakeStore) {
			fs.Dossiers["dos_2"] = &core.Dossier{Frontmatter: core.Frontmatter{ID: "dos_2", Name: "Source", Slug: "source", Status: core.StatusBlocked}, DistilledState: core.DistilledState{Body: "source"}}
			fs.Revisions["dos_2"] = "rev_2"
		}},
		{"dossier_rename", `{"id":"dos_1","new_name":"Renamed","base_revision":"stale"}`, nil},
		{"dossier_resolve_conflict", `{"conflict_id":"missing","choice":"nonsense"}`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, fs := mutationService(t, &blockingSyncer{})
			if tt.setup != nil {
				tt.setup(fs)
			}
			env, queued := mutationCall(t, svc, tt.name, tt.args)
			if env.OK {
				t.Fatalf("%s unexpectedly succeeded", tt.name)
			}
			if queued != 0 {
				t.Fatalf("%s queued %d syncs, want 0", tt.name, queued)
			}
		})
	}
}

func TestMCPReadsDoNotEnqueueMutationSync(t *testing.T) {
	reads := []struct {
		name string
		args string
	}{
		{"dossier_list", `{}`},
		{"dossier_search", `{"query":"test"}`},
		{"dossier_artifact", `{"dossier_id":"dos_1","artifact_id":"art_1"}`},
		{"dossier_artifacts", `{"dossier_id":"dos_1"}`},
		{"dossier_team", `{}`},
		{"dossier_conflicts", `{}`},
		{"dossier_session", `{}`},
	}
	for _, tt := range reads {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := mutationService(t, &blockingSyncer{})
			_, queued := mutationCall(t, svc, tt.name, tt.args)
			if queued != 0 {
				t.Fatalf("%s queued %d mutation syncs", tt.name, queued)
			}
		})
	}

	svc, _ := mutationService(t, &blockingSyncer{})
	_, queued := mutationCall(t, svc, "dossier_recall", `{"id":"dos_1"}`)
	if queued != 1 {
		t.Fatalf("dossier_recall queued %d refresh syncs, want 1", queued)
	}
}

func TestMCPMutationEnqueuesExactlyOnce(t *testing.T) {
	syncer := &blockingSyncer{doneChan: make(chan struct{}, 1)}
	svc, _ := mutationService(t, syncer)
	input := bytes.NewBufferString(
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_update","arguments":{"id":"dos_1","lead":"psmith"}},"id":1}` + "\n" +
			`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_save","arguments":{"id":"dos_1","distilled_state_markdown":"# New"}},"id":2}` + "\n")
	server := NewServer(svc, input, &safeWriter{})
	if err := server.Run(context.Background()); err != nil {
		t.Fatalf("server.Run: %v", err)
	}
	if got := atomic.LoadInt32(&syncer.syncCalls); got != 1 {
		t.Fatalf("coalesced %d sync calls, want 1", got)
	}
}

func TestMCPMutationSkipsSyncWhenTeamNotConfigured(t *testing.T) {
	svc, fs := mutationService(t, nil)
	env, queued := mutationCall(t, svc, "dossier_update", `{"id":"dos_1","lead":"psmith"}`)
	if !env.OK || queued != 0 {
		t.Fatalf("unconfigured update = %+v, queued=%d", env, queued)
	}
	if fs.Dossiers["dos_1"].Frontmatter.Lead != "psmith" {
		t.Fatal("unconfigured update did not persist")
	}
}

// A single-user store has no syncer, so an enqueued sync would fail with "team
// sync is not configured" and surface a warning about a feature the user never
// enabled. Drive the real debouncer to prove nothing is enqueued: asserting on
// the tool response alone cannot fail, because the warning is only ever set
// from the debouncer goroutine.
func TestMCPUnconfiguredStoreEmitsNoBackgroundSyncWarning(t *testing.T) {
	svc, _ := mutationService(t, nil)
	server := NewServer(svc, bytes.NewBuffer(nil), &safeWriter{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.syncDebouncer(ctx)

	server.handleRequest(ctx, JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  []byte(`{"name":"dossier_update","arguments":{"id":"dos_1","lead":"psmith"}}`),
		ID:      1,
	})

	close(server.syncChan)
	select {
	case <-server.doneChan:
	case <-time.After(5 * time.Second):
		t.Fatal("debouncer did not drain")
	}
	if w := server.takeBgWarning(); w != "" {
		t.Fatalf("unconfigured store emitted background sync warning: %q", w)
	}
}

// mcpNonMutatingTools is the other half of the classification. Every tool must
// appear in exactly one of the two sets, so a newly added tool cannot ship
// unclassified — which for a mutator means a silently stale teammate clone.
var mcpNonMutatingTools = map[string]bool{
	"dossier_list":      true,
	"dossier_recall":    true,
	"dossier_search":    true,
	"dossier_artifact":  true,
	"dossier_artifacts": true,
	"dossier_session":   true,
	"dossier_team":      true,
	"dossier_conflicts": true,
}

func TestMCPEveryToolIsClassifiedForSync(t *testing.T) {
	defined := map[string]bool{}
	for _, def := range getToolDefinitions() {
		defined[def.Name] = true
		mutating, nonMutating := mcpMutatingTools[def.Name], mcpNonMutatingTools[def.Name]
		if mutating == nonMutating {
			t.Errorf("tool %s is classified as mutating=%t and non-mutating=%t; it must be exactly one (add it to mcpMutatingTools if a successful call writes shared state)", def.Name, mutating, nonMutating)
		}
	}
	for name := range mcpMutatingTools {
		if !defined[name] {
			t.Errorf("mcpMutatingTools lists %s, which getToolDefinitions no longer defines", name)
		}
	}
	for name := range mcpNonMutatingTools {
		if !defined[name] {
			t.Errorf("mcpNonMutatingTools lists %s, which getToolDefinitions no longer defines", name)
		}
	}
}

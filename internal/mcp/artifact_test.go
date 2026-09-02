package mcp

import (
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"encoding/json"
	"strings"
	"testing"
)

// callTools drives the server over a canned request sequence and returns the
// decoded result payload of each tools/call response.
func callTools(t *testing.T, svc *core.Service, requests []string) []map[string]any {
	t.Helper()

	inBuf := bytes.NewBufferString(strings.Join(requests, "\n") + "\n")
	var outBuf bytes.Buffer
	server := NewServer(svc, inBuf, &outBuf)

	if err := server.Run(context.Background()); err != nil && err.Error() != "EOF" {
		t.Fatalf("server.Run failed: %v", err)
	}

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(outBuf.String()), "\n") {
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response failed: %v (%s)", err, line)
		}
		var res map[string]any
		_ = json.Unmarshal(resp.Result, &res)
		out = append(out, res)
	}
	return out
}

// resultText extracts the text payload from a tools/call result.
func resultText(t *testing.T, res map[string]any) string {
	t.Helper()
	list, ok := res["content"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("result has no content: %+v", res)
	}
	item, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected content item: %+v", list[0])
	}
	text, _ := item["text"].(string)
	return text
}

func newArtifactFixture(t *testing.T) *core.Service {
	t.Helper()

	fakeStore := store.NewFakeStore()
	clk := &mockClock{}
	svc := core.NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, clk,
		core.Config{}, nil)

	fakeStore.Dossiers["dos_1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_1", Name: "Test Dossier", Slug: "test-dossier",
			Status: core.StatusActive, Priority: core.PriorityMax,
			CreatedAt: clk.Now(), UpdatedAt: clk.Now(),
		},
		DistilledState: core.DistilledState{
			Body: "## Findings\n- [observed] Timeout at 200ms. [src:art_bench#L2-L3]",
		},
	}
	fakeStore.Revisions["dos_1"] = "rev_1"
	fakeStore.Artifacts["dos_1"] = []core.Artifact{
		{
			ID: "art_bench", DossierID: "dos_1", Type: core.ArtifactTypeDecisionEvidence,
			Title: "Lock benchmark", ContentFormat: core.ContentFormatText,
			Provenance: core.Provenance{Origin: "benchmark"}, CapturedAt: clk.Now(),
			Content: "alpha\nbravo\ncharlie\ndelta\n",
		},
		{
			ID: "art_orphan", DossierID: "dos_1", Type: core.ArtifactTypeTranscript,
			Title: "Session", ContentFormat: core.ContentFormatText,
			Provenance: core.Provenance{Origin: "session"}, CapturedAt: clk.Now(),
			Content: "unreferenced\n",
		},
	}
	return svc
}

func TestDossierArtifactToolIsAdvertised(t *testing.T) {
	names := map[string]bool{}
	for _, def := range getToolDefinitions() {
		names[def.Name] = true
	}
	for _, want := range []string{"dossier_artifact", "dossier_artifacts"} {
		if !names[want] {
			t.Errorf("tool %q is not advertised in tools/list", want)
		}
	}
}

func TestDossierArtifactResolvesCitedRange(t *testing.T) {
	svc := newArtifactFixture(t)

	results := callTools(t, svc, []string{
		`{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05"},"id":1}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_artifact","arguments":{"dossier_id":"dos_1","artifact_id":"art_bench","fragment":"L2-L3"}},"id":2}`,
	})

	text := resultText(t, results[1])
	// The envelope is JSON, so the line-number separator arrives escaped.
	if !strings.Contains(text, `2\tbravo`) || !strings.Contains(text, `3\tcharlie`) {
		t.Errorf("cited range did not resolve to its verbatim lines:\n%s", text)
	}
	if strings.Contains(text, "delta") {
		t.Errorf("response leaked content outside the cited range:\n%s", text)
	}
}

func TestDossierArtifactsListsEvidenceIndexWithCitationState(t *testing.T) {
	svc := newArtifactFixture(t)

	results := callTools(t, svc, []string{
		`{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05"},"id":1}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_artifacts","arguments":{"dossier_id":"dos_1"}},"id":2}`,
	})

	text := resultText(t, results[1])
	for _, want := range []string{"art_bench", "art_orphan", `"cited":true`, `"cited":false`} {
		if !strings.Contains(text, want) {
			t.Errorf("evidence index missing %q:\n%s", want, text)
		}
	}
}

func TestDossierRecallCarriesEvidenceIndex(t *testing.T) {
	svc := newArtifactFixture(t)

	results := callTools(t, svc, []string{
		`{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05"},"id":1}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_recall","arguments":{"id":"dos_1"}},"id":2}`,
	})

	text := resultText(t, results[1])
	if !strings.Contains(text, "art_bench") {
		t.Errorf("recall did not carry the evidence index:\n%s", text)
	}
	if !strings.Contains(text, "art_orphan") {
		t.Errorf("recall did not surface the uncited artifact:\n%s", text)
	}
}

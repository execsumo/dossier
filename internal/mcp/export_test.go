package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dossier/internal/core"
	"dossier/internal/store"
)

type mcpExportFixture struct {
	svc       *core.Service
	fake      *store.FakeStore
	userHome  string
	storeHome string
}

func newMCPExportFixture(t *testing.T) *mcpExportFixture {
	t.Helper()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	t.Setenv("DOSSIER_HOME", "")
	t.Chdir(t.TempDir())

	storeHome := filepath.Join(t.TempDir(), "store")
	fake := store.NewFakeStore()
	clk := &mockClock{}
	svc := core.NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, clk,
		core.Config{DossierHome: storeHome, Author: "alice"}, nil)
	fake.Dossiers["dos_1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_1", Name: "Test Dossier", Slug: "test-dossier", Status: core.StatusActive, Priority: core.PriorityHigh,
			CreatedAt: clk.Now(), UpdatedAt: clk.Now(),
		},
		DistilledState: core.DistilledState{Body: "## Findings\n- Timeout at 200ms. [src:art_a]\n"},
	}
	fake.Revisions["dos_1"] = "rev_1"
	fake.Artifacts["dos_1"] = []core.Artifact{
		{ID: "art_a", DossierID: "dos_1", Type: core.ArtifactTypeSourceSnapshot, Title: "Bench", CapturedAt: clk.Now(), RefreshedAt: clk.Now(), ContentFormat: core.ContentFormatText, Content: "bench output\n"},
		{ID: "art_t", DossierID: "dos_1", Type: core.ArtifactTypeTranscript, Title: "Session", CapturedAt: clk.Now(), RefreshedAt: clk.Now(), ContentFormat: core.ContentFormatText, Content: "TRANSCRIPT-SENTINEL"},
	}
	fake.WorkingFiles = map[string]map[string][]byte{"dos_1": {"files/notes.md": []byte("working notes\n")}}
	return &mcpExportFixture{svc: svc, fake: fake, userHome: userHome, storeHome: storeHome}
}

func dataMap(t *testing.T, env mcpEnvelope) map[string]any {
	t.Helper()
	b, _ := json.Marshal(env.Data)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMCPExportIsRegisteredWithSchema(t *testing.T) {
	var found *ToolDefinition
	for _, def := range getToolDefinitions() {
		if def.Name == "dossier_export" {
			d := def
			found = &d
		}
	}
	if found == nil {
		t.Fatal("dossier_export is not in the tool list")
	}
	props, _ := found.InputSchema["properties"].(map[string]any)
	for _, p := range []string{"id", "output_path", "force", "inline"} {
		if _, ok := props[p]; !ok {
			t.Errorf("schema missing %q", p)
		}
	}
}

func TestMCPExportDefaultLocationAndEnvelope(t *testing.T) {
	f := newMCPExportFixture(t)
	if err := os.Mkdir(filepath.Join(f.userHome, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := callTool(t, f.svc, "dossier_export", `{"id":"test-dossier"}`)
	if !env.OK || env.Error != nil {
		t.Fatalf("export failed: %+v", env.Error)
	}
	d := dataMap(t, env)
	want := filepath.Join(f.userHome, "Downloads", "test-dossier-export-2026-06-14.md")
	if d["output"] != want || d["revision"] != "rev_1" {
		t.Fatalf("data = %v", d)
	}
	if _, has := d["markdown"]; has {
		t.Fatalf("the document must not be returned inline by default")
	}
	doc, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(doc), "TRANSCRIPT-SENTINEL") || !strings.Contains(string(doc), "working notes\n") || !strings.Contains(string(doc), "bench output\n") {
		t.Fatalf("unexpected contents:\n%s", doc)
	}
	if ids, _ := d["artifacts_included"].([]any); len(ids) != 1 || ids[0] != "art_a" {
		t.Fatalf("artifacts_included = %v", d["artifacts_included"])
	}

	// Second same-day export gets -2 and leaves the first alone.
	first, _ := os.ReadFile(want)
	env = callTool(t, f.svc, "dossier_export", `{"id":"test-dossier"}`)
	if got := dataMap(t, env)["output"]; got != strings.TrimSuffix(want, ".md")+"-2.md" {
		t.Fatalf("second export output = %v", got)
	}
	if again, _ := os.ReadFile(want); string(again) != string(first) {
		t.Fatalf("first export modified")
	}

	events := f.fake.Audits["dos_1"]
	if len(events) != 2 || events[0].Event != "exported" || events[0].Output != filepath.Base(want) {
		t.Fatalf("audit events = %+v", events)
	}
	if f.fake.Revisions["dos_1"] != "rev_1" {
		t.Fatal("revision changed")
	}
}

func TestMCPExportExplicitPathForceInlineAndRejections(t *testing.T) {
	f := newMCPExportFixture(t)
	out := filepath.Join(t.TempDir(), "brief.md")

	env := callTool(t, f.svc, "dossier_export", `{"id":"dos_1","output_path":`+quote(out)+`,"inline":true}`)
	if !env.OK {
		t.Fatalf("export failed: %+v", env.Error)
	}
	d := dataMap(t, env)
	written, _ := os.ReadFile(out)
	if d["markdown"] != string(written) || len(written) == 0 {
		t.Fatalf("inline document must equal the written file")
	}

	env = callTool(t, f.svc, "dossier_export", `{"id":"dos_1","output_path":`+quote(out)+`}`)
	if env.OK || env.Error == nil || !strings.Contains(env.Error.Message, "force") {
		t.Fatalf("existing file without force must be refused: %+v", env)
	}
	if err := os.WriteFile(out, []byte("PRECIOUS"), 0o644); err != nil {
		t.Fatal(err)
	}
	env = callTool(t, f.svc, "dossier_export", `{"id":"dos_1","output_path":`+quote(out)+`}`)
	if env.OK {
		t.Fatal("expected refusal")
	}
	if b, _ := os.ReadFile(out); string(b) != "PRECIOUS" {
		t.Fatalf("refused export modified the file: %q", b)
	}
	env = callTool(t, f.svc, "dossier_export", `{"id":"dos_1","output_path":`+quote(out)+`,"force":true}`)
	if !env.OK {
		t.Fatalf("force export failed: %+v", env.Error)
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), "# Test Dossier") {
		t.Fatalf("force did not overwrite")
	}
	if names, _ := os.ReadDir(filepath.Dir(out)); len(names) != 1 {
		t.Fatalf("temp files left behind: %v", names)
	}

	inside := filepath.Join(f.storeHome, "test-dossier", "brief.md")
	env = callTool(t, f.svc, "dossier_export", `{"id":"dos_1","output_path":`+quote(inside)+`,"force":true}`)
	if env.OK || env.Error == nil || !strings.Contains(env.Error.Message, "inside the Dossier store") {
		t.Fatalf("a path inside the store must be rejected: %+v", env)
	}
	if _, err := os.Stat(filepath.Dir(inside)); !os.IsNotExist(err) {
		t.Fatalf("rejected export created %s", filepath.Dir(inside))
	}

	if env := callTool(t, f.svc, "dossier_export", `{"id":"dos_missing"}`); env.OK || env.Error == nil {
		t.Fatalf("unknown dossier must fail: %+v", env)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

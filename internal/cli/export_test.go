package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
	"dossier/internal/mcp"
	"dossier/internal/store"
)

const exportBody = "## Objective\nShip pricing. [src:art_cited]\n\n## Constraints\n- Budget is fixed. Data in /home/alice/pricing/data.csv\n\n## Files\n- `files/writeup.md`: formal write-up\n- `files/deck.pptx`: slides\n"

type exportEnv struct {
	home    string // DOSSIER_HOME
	userHom string // fake HOME
	fs      *store.FSStore
	dir     string // dossier directory
	rev     core.Revision
}

// newExportEnv builds a real on-disk store with a Dossier that has evidence,
// working files, and sentinel data in every namespace export must never read.
// HOME points at a temp dir so nothing touches the real ~/Downloads, and the
// working directory is moved away from the repo.
func newExportEnv(t *testing.T, withDownloads bool) *exportEnv {
	t.Helper()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	t.Setenv("DOSSIER_HOME", "")
	t.Chdir(t.TempDir())
	if withDownloads {
		if err := os.Mkdir(filepath.Join(userHome, "Downloads"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	home := filepath.Join(t.TempDir(), "store")
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	d := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_exp", Name: "Pricing Review", Slug: "pricing-review",
			CreatedAt: now, UpdatedAt: now, Status: core.StatusActive, Priority: core.PriorityMedium,
		},
		DistilledState: core.DistilledState{Body: "## Objective\nHISTORY-SENTINEL old body\n"},
	}
	rev, err := fs.Write(d, "")
	if err != nil {
		t.Fatal(err)
	}
	d.DistilledState.Body = exportBody
	if rev, err = fs.Write(d, rev); err != nil {
		t.Fatal(err)
	}

	art := func(id string, typ core.ArtifactType, title, content string) {
		t.Helper()
		if err := fs.WriteArtifact("dos_exp", &core.Artifact{
			ID: id, DossierID: "dos_exp", Type: typ, Title: title, CapturedAt: now, RefreshedAt: now,
			Provenance: core.Provenance{Origin: "test"}, ContentFormat: core.ContentFormatMarkdown, Content: content,
		}); err != nil {
			t.Fatal(err)
		}
	}
	art("art_cited", core.ArtifactTypeSourceSnapshot, "Cited source", "evidence with a ```fence``` inside\n```\nnested\n```\n")
	art("art_tr", core.ArtifactTypeTranscript, "Session transcript", "TRANSCRIPT-SENTINEL")

	dir := filepath.Join(home, "pricing-review")
	put := func(rel string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("files/writeup.md", []byte("# Write-up\nThe formal text.\n"))
	put("files/deck.pptx", append([]byte("PK\x03\x04"), 0, 0, 0))

	if err := fs.AppendAudit("dos_exp", core.AuditEvent{TS: now, Event: "save", DossierID: "dos_exp", Message: "AUDIT-SENTINEL"}); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteConflict(&core.Conflict{ID: "con_1", DossierID: "dos_exp", Kind: "concurrent_edit", TS: now, RejectedBody: "CONFLICT-SENTINEL"}); err != nil {
		t.Fatal(err)
	}
	svc, err := wire(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateInbox(context.Background(), core.InboxCreateReq{
		ID: "dos_exp", Source: core.InboxSource{Kind: "email"}, Excerpt: "INBOX-SENTINEL", RoutedBy: "agent:test", Confidence: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	// The revision a reader of the store sees is the one export must report.
	if _, rev, err = fs.Read("dos_exp"); err != nil {
		t.Fatal(err)
	}
	return &exportEnv{home: home, userHom: userHome, fs: fs, dir: dir, rev: rev}
}

func (e *exportEnv) run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := NewRootCmd()
	cmd.SetArgs(append([]string{"export"}, append(args, "--home", e.home)...))
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestExportCLIDefaultPathAndContents(t *testing.T) {
	e := newExportEnv(t, true)
	stdout, _, err := e.run(t, "pricing-review")
	if err != nil {
		t.Fatalf("export: %v\n%s", err, stdout)
	}
	date := time.Now().Format("2006-01-02")
	want := filepath.Join(e.userHom, "Downloads", "pricing-review-export-"+date+".md")
	doc := readFile(t, want)
	if !strings.Contains(stdout, want) || !strings.Contains(stdout, string(e.rev)) {
		t.Fatalf("summary must name the output path and revision:\n%s", stdout)
	}

	if !strings.Contains(doc, exportBody) {
		t.Fatalf("Distilled State not byte-for-byte present:\n%s", doc)
	}
	for _, sentinel := range []string{"HISTORY-SENTINEL", "AUDIT-SENTINEL", "CONFLICT-SENTINEL", "INBOX-SENTINEL", "TRANSCRIPT-SENTINEL"} {
		if strings.Contains(doc, sentinel) {
			t.Errorf("%s leaked into the export", sentinel)
		}
	}
	if strings.HasPrefix(doc, "---") || strings.Contains(doc, "dossier_id") {
		t.Fatalf("frontmatter leaked:\n%s", doc)
	}
	// The evidence's own ``` fences force a longer enclosing fence.
	if !strings.Contains(doc, "\n````\nevidence with a ```fence``` inside\n```\nnested\n```\n````\n") {
		t.Fatalf("nested fence did not round-trip:\n%s", doc)
	}
	if !strings.Contains(doc, "# Write-up\nThe formal text.\n") || !strings.Contains(doc, "files/deck.pptx") {
		t.Fatalf("working files not handled:\n%s", doc)
	}
	notIncluded := doc[strings.Index(doc, "## Not included"):]
	if !strings.Contains(notIncluded, "art_tr") || !strings.Contains(notIncluded, "Session transcript") || !strings.Contains(notIncluded, "files/deck.pptx") {
		t.Fatalf("exclusions not listed:\n%s", notIncluded)
	}
	for _, w := range []string{"Warning: Local path /home/alice/pricing/data.csv", "unresolved conflict", "1 transcripts, 1 binary files, 0 external file references not included."} {
		if !strings.Contains(stdout, w) {
			t.Errorf("summary missing %q:\n%s", w, stdout)
		}
	}

	// Audit: exactly one exported event, basename only, revision unchanged.
	events, err := e.fs.ReadAuditLog("dos_exp")
	if err != nil {
		t.Fatal(err)
	}
	var exported []core.AuditEvent
	for _, ev := range events {
		if ev.Event == core.AuditEventExported {
			exported = append(exported, ev)
		}
	}
	if len(exported) != 1 {
		t.Fatalf("want 1 exported event, got %d", len(exported))
	}
	ev := exported[0]
	if ev.Output != filepath.Base(want) || ev.Revision != string(e.rev) ||
		strings.Join(ev.ArtifactsIncluded, ",") != "art_cited" || strings.Join(ev.FilesIncluded, ",") != "files/writeup.md" {
		t.Fatalf("audit event wrong: %+v", ev)
	}
	if _, rev, err := e.fs.Read("dos_exp"); err != nil || rev != e.rev {
		t.Fatalf("revision changed by export: %v -> %v (%v)", e.rev, rev, err)
	}
	if names := dirNames(t, filepath.Join(e.userHom, "Downloads")); len(names) != 1 {
		t.Fatalf("stray files left beside the export: %v", names)
	}
}

func TestExportCLIDefaultPathFallsBackToHome(t *testing.T) {
	e := newExportEnv(t, false)
	if _, _, err := e.run(t, "pricing-review"); err != nil {
		t.Fatal(err)
	}
	date := time.Now().Format("2006-01-02")
	if _, err := os.Stat(filepath.Join(e.userHom, "pricing-review-export-"+date+".md")); err != nil {
		t.Fatalf("expected the export in the home directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.userHom, "Downloads")); !os.IsNotExist(err) {
		t.Fatalf("~/Downloads must not be created")
	}
}

func TestExportCLICollisionSuffixLeavesFirstUntouched(t *testing.T) {
	e := newExportEnv(t, true)
	date := time.Now().Format("2006-01-02")
	dl := filepath.Join(e.userHom, "Downloads")
	first := filepath.Join(dl, "pricing-review-export-"+date+".md")

	if _, _, err := e.run(t, "pricing-review"); err != nil {
		t.Fatal(err)
	}
	firstBytes := readFile(t, first)
	// Mutate the first file to prove a later export never touches it.
	if err := os.WriteFile(first, []byte(firstBytes+"HAND-EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-2", "-3"} {
		if _, _, err := e.run(t, "pricing-review"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dl, "pricing-review-export-"+date+suffix+".md")); err != nil {
			t.Fatalf("expected %s export: %v (have %v)", suffix, err, dirNames(t, dl))
		}
	}
	if got := readFile(t, first); got != firstBytes+"HAND-EDIT" {
		t.Fatalf("the first export was overwritten")
	}
	if names := dirNames(t, dl); len(names) != 3 {
		t.Fatalf("want exactly 3 files, got %v", names)
	}
}

func TestExportCLIExplicitPathForce(t *testing.T) {
	e := newExportEnv(t, true)
	out := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(out, []byte("PRECIOUS"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := e.run(t, "pricing-review", "-o", out); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("existing explicit path must be refused without --force, got %v", err)
	}
	if got := readFile(t, out); got != "PRECIOUS" {
		t.Fatalf("refused export modified the file: %q", got)
	}
	if names := dirNames(t, filepath.Dir(out)); len(names) != 1 {
		t.Fatalf("refused export left files behind: %v", names)
	}
	if events, _ := e.fs.ReadAuditLog("dos_exp"); hasExportedEvent(events) {
		t.Fatalf("a refused export must not be audited")
	}

	if _, _, err := e.run(t, "pricing-review", "-o", out, "--force"); err != nil {
		t.Fatalf("--force export: %v", err)
	}
	if got := readFile(t, out); !strings.Contains(got, "# Pricing Review") {
		t.Fatalf("--force did not replace the file: %q", got)
	}
	if names := dirNames(t, filepath.Dir(out)); len(names) != 1 {
		t.Fatalf("temp files left behind: %v", names)
	}

	fresh := filepath.Join(t.TempDir(), "new.md")
	if _, _, err := e.run(t, "pricing-review", "-o", fresh); err != nil {
		t.Fatal(err)
	}
}

func hasExportedEvent(events []core.AuditEvent) bool {
	for _, ev := range events {
		if ev.Event == core.AuditEventExported {
			return true
		}
	}
	return false
}

func TestExportCLIRejectsPathsInsideStoreAndBadTargets(t *testing.T) {
	e := newExportEnv(t, true)
	link := filepath.Join(t.TempDir(), "link-to-store")
	if err := os.Symlink(e.home, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cases := map[string]string{
		"store root file":       filepath.Join(e.home, "brief.md"),
		"inside a dossier":      filepath.Join(e.dir, "files", "brief.md"),
		"inside archive":        filepath.Join(e.home, "archive", "brief.md"),
		"store root itself":     e.home,
		"through a symlink":     filepath.Join(link, "brief.md"),
		"dotdot into the store": filepath.Join(t.TempDir(), "..", filepath.Base(filepath.Dir(e.home)), "store", "x.md"),
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := e.run(t, "pricing-review", "-o", target, "--force")
			if err == nil || !strings.Contains(err.Error(), "inside the Dossier store") {
				t.Fatalf("want a store-path rejection, got %v", err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(e.home, "brief.md")); !os.IsNotExist(err) {
		t.Fatalf("a rejected export created a file in the store")
	}
	if events, _ := e.fs.ReadAuditLog("dos_exp"); hasExportedEvent(events) {
		t.Fatalf("rejected exports must not be audited")
	}

	t.Run("missing directory leaves nothing", func(t *testing.T) {
		parent := t.TempDir()
		_, _, err := e.run(t, "pricing-review", "-o", filepath.Join(parent, "nope", "brief.md"))
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("got %v", err)
		}
		if names := dirNames(t, parent); len(names) != 0 {
			t.Fatalf("left behind %v", names)
		}
	})
	t.Run("directory target", func(t *testing.T) {
		dir := t.TempDir()
		_, _, err := e.run(t, "pricing-review", "-o", dir, "--force")
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unknown dossier", func(t *testing.T) {
		if _, _, err := e.run(t, "no-such-dossier"); err == nil {
			t.Fatal("expected not_found")
		}
	})
}

func TestExportCLIStdoutAndJSON(t *testing.T) {
	e := newExportEnv(t, true)
	stdout, stderr, err := e.run(t, "pricing-review", "-o", "-")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout, "# Pricing Review\n") || !strings.Contains(stdout, exportBody) {
		t.Fatalf("stdout must be exactly the document:\n%s", stdout)
	}
	if strings.Contains(stdout, "Revision:") || !strings.Contains(stderr, "Revision: "+string(e.rev)) || !strings.Contains(stderr, "Warning:") {
		t.Fatalf("summary belongs on stderr only.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if names := dirNames(t, filepath.Join(e.userHom, "Downloads")); len(names) != 0 {
		t.Fatalf("-o - wrote a file: %v", names)
	}
	events, _ := e.fs.ReadAuditLog("dos_exp")
	var got string
	for _, ev := range events {
		if ev.Event == core.AuditEventExported {
			got = ev.Output
		}
	}
	if got != "-" {
		t.Fatalf("stdout export must audit output %q, got %q", "-", got)
	}

	out := filepath.Join(t.TempDir(), "j.md")
	jsonOut, _, err := e.run(t, "pricing-review", "-o", out, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		OK   bool              `json:"ok"`
		Data core.ExportResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &env); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, jsonOut)
	}
	if !env.OK || env.Data.Output != out || env.Data.Revision != e.rev || env.Data.Markdown != "" || len(env.Data.Excluded) != 2 {
		t.Fatalf("unexpected JSON summary: %+v", env)
	}
}

// TestExportCLIAndMCPProduceIdenticalDocuments covers SPEC §14.13: both
// adapters route through one Service.Export and the shared writer.
func TestExportCLIAndMCPProduceIdenticalDocuments(t *testing.T) {
	e := newExportEnv(t, true)
	cliOut := filepath.Join(t.TempDir(), "cli.md")
	if _, _, err := e.run(t, "pricing-review", "-o", cliOut); err != nil {
		t.Fatal(err)
	}

	svc, err := wire(e.home)
	if err != nil {
		t.Fatal(err)
	}
	mcpOut := filepath.Join(t.TempDir(), "mcp.md")
	call := func(args string) map[string]any {
		t.Helper()
		in := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_export","arguments":` + args + `},"id":1}` + "\n")
		var out bytes.Buffer
		if err := mcp.NewServer(svc, in, &out).Run(context.Background()); err != nil && err.Error() != "EOF" {
			t.Fatal(err)
		}
		var resp struct {
			Result struct {
				Content []struct{ Text string } `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
			t.Fatalf("%v: %s", err, out.String())
		}
		var env map[string]any
		if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &env); err != nil {
			t.Fatal(err)
		}
		return env
	}
	argsJSON, _ := json.Marshal(map[string]any{"id": "pricing-review", "output_path": mcpOut, "inline": true})
	env := call(string(argsJSON))
	if env["ok"] != true {
		t.Fatalf("mcp export failed: %v", env)
	}
	if a, b := readFile(t, cliOut), readFile(t, mcpOut); a != b {
		t.Fatalf("CLI and MCP documents differ")
	}
	data := env["data"].(map[string]any)
	if data["markdown"] != readFile(t, mcpOut) {
		t.Fatalf("inline markdown must equal the written file")
	}
	if data["output"] != mcpOut {
		t.Fatalf("output = %v", data["output"])
	}
}

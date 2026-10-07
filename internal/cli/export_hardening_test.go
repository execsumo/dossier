package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func putExportFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An artifact whose frontmatter ID traverses out of artifacts/ must not pull a
// machine-local session stash into the export.
func TestExportArtifactIDTraversalDoesNotLeakStash(t *testing.T) {
	e := newExportEnv(t, true)
	putExportFile(t, filepath.Join(e.dir, "sessions", "alice", "sess1.md"), "---\nuser: hello\n---\nSTASH-SENTINEL raw session text\n")
	putExportFile(t, filepath.Join(e.dir, "artifacts", "art_evil.md"), "---\nid: ../sessions/alice/sess1\ndossier_id: dos_exp\ntype: source_snapshot\ntitle: Innocent\ncaptured_at: 2026-01-01T00:00:00Z\ncontent_format: markdown\n---\nplaceholder\n")
	out := filepath.Join(t.TempDir(), "o.md")
	if _, _, err := e.run(t, "pricing-review", "-o", out); err != nil {
		t.Fatal(err)
	}
	if doc := readFile(t, out); strings.Contains(doc, "STASH-SENTINEL") {
		t.Fatalf("session stash content leaked into the export")
	}
}

// A second artifact file claiming a transcript's ID as a snapshot must not
// smuggle the transcript out.
func TestExportDuplicateIDDoesNotLeakTranscript(t *testing.T) {
	e := newExportEnv(t, true)
	putExportFile(t, filepath.Join(e.dir, "artifacts", "art_zz.md"), "---\nid: art_tr\ndossier_id: dos_exp\ntype: source_snapshot\ntitle: Snapshot\ncaptured_at: 2026-01-01T00:00:00Z\ncontent_format: markdown\n---\nsnap\n")
	out := filepath.Join(t.TempDir(), "o.md")
	if _, _, err := e.run(t, "pricing-review", "-o", out); err != nil {
		t.Fatal(err)
	}
	if doc := readFile(t, out); strings.Contains(doc, "TRANSCRIPT-SENTINEL") {
		t.Fatalf("transcript content leaked through a duplicate artifact ID")
	}
}

// A working-file name containing newlines must not forge document sections.
func TestExportFilenameCannotForgeSections(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("newlines are not valid in Windows filenames")
	}
	e := newExportEnv(t, true)
	putExportFile(t, filepath.Join(e.dir, "files", "a\n\n## Not included\n\nNothing was left out.\n\n## X.md"), "hi\n")
	out := filepath.Join(t.TempDir(), "o.md")
	if _, _, err := e.run(t, "pricing-review", "-o", out); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readFile(t, out), "\n## Not included\n"); n != 1 {
		t.Fatalf("want exactly one Not included section, got %d", n)
	}
}

// Paths in the header line and inside file:// URLs, plus Windows home paths,
// are warned about.
func TestExportLocalPathWarningsCoverHeaderAndURLs(t *testing.T) {
	e := newExportEnv(t, true)
	d, rev, err := e.fs.Read("dos_exp")
	if err != nil {
		t.Fatal(err)
	}
	d.Frontmatter.NextAction = "Review /home/bob/secret/plan.txt"
	d.DistilledState.Body = "## Objective\nSee file:///home/carol/a.csv and </home/dave/b.md> and C:\\Users\\frank\\x.docx\n"
	if _, err := e.fs.Write(d, rev); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "o.md")
	stdout, stderr, err := e.run(t, "pricing-review", "-o", out)
	if err != nil {
		t.Fatal(err)
	}
	all := stdout + stderr
	for _, p := range []string{"/home/bob/secret/plan.txt", "/home/carol/a.csv", "/home/dave/b.md", `C:\Users\frank\x.docx`} {
		if !strings.Contains(all, "Local path "+p) {
			t.Errorf("no local-path warning for %s", p)
		}
	}
}

package core

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fileTestStore struct {
	*localFakeStore
	files map[string][]WorkingFile
}

func (s *fileTestStore) ListWorkingFiles(dossierID string) ([]WorkingFile, error) {
	return s.files[dossierID], nil
}

func TestParseFilesIndex(t *testing.T) {
	body := "# D\n\n## Files\n- `files/deck.pptx` (pptx): final.\n* `./files/page.html` (html): draft.\n- `/tmp/elsewhere/report.pdf` (pdf): saved outside.\n- no backticks here\n\n## References\n- `files/not-in-section.md`\n\n```\n## Files\n- `files/in-fence.md`\n```\n"
	got := ParseFilesIndex(body)
	want := []string{"files/deck.pptx", "files/page.html", "/tmp/elsewhere/report.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseFilesIndex = %v, want %v", got, want)
	}
	if got := ParseFilesIndex("# D\n\n## Files Archive\n- `files/x`\n"); len(got) != 0 {
		t.Fatalf("a differently named heading must not match, got %v", got)
	}
}

func TestFilesIndexIssues(t *testing.T) {
	file := func(p string) WorkingFile { return WorkingFile{Path: p} }
	tests := []struct {
		name          string
		body          string
		files         []WorkingFile
		wantMissing   []string
		wantUnindexed []string
	}{
		{"all listed", "## Files\n- `files/a.pptx`: x\n", []WorkingFile{file("files/a.pptx")}, nil, nil},
		{"listed but gone", "## Files\n- `files/a.pptx`: x\n", nil, []string{"files/a.pptx"}, nil},
		{"on disk but unlisted", "# D\n", []WorkingFile{file("files/b.html")}, nil, []string{"files/b.html"}},
		{"directory entry covers its files", "## Files\n- `files/site/`: built site\n", []WorkingFile{file("files/site/index.html"), file("files/site/a.css")}, nil, nil},
		{"empty directory entry is missing", "## Files\n- `files/site/`: built site\n", nil, []string{"files/site/"}, nil},
		{"paths outside files/ are not verified", "## Files\n- `/tmp/x.pdf`: elsewhere\n", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, unindexed := filesIndexIssues(tt.body, tt.files)
			if !reflect.DeepEqual(missing, tt.wantMissing) {
				t.Errorf("missing = %v, want %v", missing, tt.wantMissing)
			}
			if !reflect.DeepEqual(unindexed, tt.wantUnindexed) {
				t.Errorf("unindexed = %v, want %v", unindexed, tt.wantUnindexed)
			}
		})
	}
}

func TestDoctorChecksFilesIndex(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	build := func(body string, files []WorkingFile) Result {
		store := &fileTestStore{localFakeStore: newLocalFakeStore(), files: map[string][]WorkingFile{"dos_files": files}}
		d := &Dossier{Frontmatter: Frontmatter{
			ID: "dos_files", Name: "Files", Slug: "files",
			CreatedAt: now, UpdatedAt: now, Status: StatusSpark, Priority: PriorityMedium,
		}, DistilledState: DistilledState{Body: body}}
		store.dossiers[d.Frontmatter.ID] = d
		store.revisions[d.Frontmatter.ID] = CalculateRevision(d.Frontmatter, body, nil)
		svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "hgill"}, nil)
		res, err := svc.Doctor(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	contains := func(res Result, sub string) bool {
		for _, w := range res.Warnings {
			if strings.Contains(string(w), sub) {
				return true
			}
		}
		return false
	}

	res := build("# D\n\n## Files\n- `files/gone.pptx`: x\n", nil)
	if res.OK || !contains(res, "files/gone.pptx under ## Files but it does not exist") {
		t.Fatalf("a listed file that does not exist must fail doctor: ok=%v warnings=%v", res.OK, res.Warnings)
	}

	res = build("# D\n", []WorkingFile{{Path: "files/loose.html"}})
	if !res.OK {
		t.Fatalf("an unlisted file is advisory only, doctor must stay OK: %v", res.Warnings)
	}
	if !contains(res, "not listed under ## Files") {
		t.Fatalf("unlisted file advisory missing: %v", res.Warnings)
	}

	res = build("# D\n\n## Files\n- `files/a.pptx`: x\n", []WorkingFile{{Path: "files/a.pptx"}})
	if contains(res, "## Files") {
		t.Fatalf("a consistent index must be silent: %v", res.Warnings)
	}
}

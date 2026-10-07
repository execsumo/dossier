package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAddReference(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		body    string
		req     AddReferenceReq
		want    string // substring of resulting body
		wantErr bool
	}{
		{
			name: "creates the section",
			body: "# Routes\n\n## Constraints\n- none\n",
			req:  AddReferenceReq{URL: "https://example.test/a", Label: "Spec", Kind: "Doc", Description: "The spec"},
			want: "## Constraints\n- none\n\n## References\n\n- [doc: Spec](https://example.test/a) — The spec\n",
		},
		{
			name: "appends to an existing section before the next heading",
			body: "## References\n- [ticket: OPS-7](https://example.test/7) — Ticket.\n\n## Active Monitors\n- [comms: #ops](https://example.test/ops) — Watch.\n",
			req:  AddReferenceReq{URL: "https://example.test/8"},
			want: "— Ticket.\n- [link: example.test](https://example.test/8) —\n\n## Active Monitors",
		},
		{
			name: "fills an empty section",
			body: "## References\n\n## Notes\nx\n",
			req:  AddReferenceReq{URL: "https://example.test/8", Label: "Eight"},
			want: "## References\n\n- [link: Eight](https://example.test/8) —\n",
		},
		{name: "duplicate", body: "## References\n- [ticket: A](https://example.test/7) — x\n", req: AddReferenceReq{URL: "https://example.test/7"}, wantErr: true},
		{name: "non-http", body: "x\n", req: AddReferenceReq{URL: "ftp://example.test/7"}, wantErr: true},
		{name: "bad label", body: "x\n", req: AddReferenceReq{URL: "https://example.test/7", Label: "a]b"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newLocalFakeStore()
			fake.dossiers["dos_r"] = &Dossier{
				Frontmatter:    Frontmatter{ID: "dos_r", Name: "R", Slug: "r", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now},
				DistilledState: DistilledState{Body: tc.body},
			}
			svc := NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
			tc.req.ID = "dos_r"
			_, err := svc.AddReference(context.Background(), tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if got := fake.dossiers["dos_r"].DistilledState.Body; got != tc.body {
					t.Fatalf("body changed on error: %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := fake.dossiers["dos_r"].DistilledState.Body
			if !strings.Contains(got, tc.want) {
				t.Fatalf("body = %q, want substring %q", got, tc.want)
			}
			if refs := ParseExternalLinks(got).References; len(refs) == 0 || refs[len(refs)-1].URL != tc.req.URL {
				t.Fatalf("added reference does not round-trip through ParseExternalLinks: %+v", refs)
			}
		})
	}
}

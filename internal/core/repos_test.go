package core

import "testing"

func TestNormalizeRepo(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"git@github.com:acme/billing-api.git", "github.com/acme/billing-api"},
		{"ssh://git@github.com/acme/billing-api", "github.com/acme/billing-api"},
		{"ssh://git@github.com:22/acme/billing-api.git", "github.com/acme/billing-api"},
		{"https://github.com/Acme/Billing-API", "github.com/acme/billing-api"},
		{"https://user:token@github.com/acme/x.git/", "github.com/acme/x"},
		{"http://gitlab.example.com:8080/group/sub/proj.git", "gitlab.example.com/group/sub/proj"},
		{"github.com/acme/x", "github.com/acme/x"},
		{"  GitHub.com/acme/x/  ", "github.com/acme/x"},
		{"git://github.com/acme/x", "github.com/acme/x"},
	} {
		got, err := NormalizeRepo(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeRepo(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "acme", "github.com", "github.com/acme", "https://github.com/", "github.com/acme/../x", "github.com/acme/x y", "/home/u/repo", "C:\\src\\x"} {
		if got, err := NormalizeRepo(bad); err == nil {
			t.Errorf("NormalizeRepo(%q) = %q, want an error", bad, got)
		}
	}
}

func TestNormalizeRepoList(t *testing.T) {
	got, err := NormalizeRepoList([]string{"git@github.com:acme/a.git", "github.com/acme/b"})
	if err != nil || len(got) != 2 || got[0] != "github.com/acme/a" || got[1] != "github.com/acme/b" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := NormalizeRepoList([]string{"github.com/acme/a", "https://github.com/acme/a.git"}); err == nil {
		t.Error("duplicates after normalization must be rejected")
	}
}

func TestRepoRef(t *testing.T) {
	if got := RepoRef("github.com/acme/a", "docs/x.md"); got != "github.com/acme/a:docs/x.md" {
		t.Errorf("RepoRef = %q", got)
	}
}

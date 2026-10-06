package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Repo identity and local resolution (ADR 0015). A Dossier names the repos its
// work lives in by normalized remote identity — "host/owner/name" — never by
// path, because every teammate keeps checkouts somewhere different. Each machine
// resolves identities to local paths through a RepoLocator.

// NormalizeRepo canonicalizes a git remote or identity to lowercase
// "host/owner/name". It accepts scheme URLs (https://, ssh://, git://),
// scp-style remotes (git@host:owner/name.git) and bare identities, and drops
// the user, port, a trailing ".git" and trailing slashes. Subgroup paths
// (host/group/sub/name) are kept whole.
func NormalizeRepo(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", fmt.Errorf("repo must not be empty")
	}
	var host, path string
	switch {
	case strings.Contains(s, "://"):
		// scheme://[user@]host[:port]/path — parsed by hand: core stays free of net/*.
		_, rest, _ := strings.Cut(s, "://")
		authority, p, _ := strings.Cut(rest, "/")
		if i := strings.LastIndex(authority, "@"); i >= 0 {
			authority = authority[i+1:]
		}
		if h, _, hasPort := strings.Cut(authority, ":"); hasPort {
			authority = h
		}
		host, path = authority, p
	case strings.Contains(s, ":") && !strings.Contains(strings.SplitN(s, ":", 2)[0], "/"):
		// scp-style: [user@]host:owner/name
		hostPart, rest, _ := strings.Cut(s, ":")
		if i := strings.LastIndex(hostPart, "@"); i >= 0 {
			hostPart = hostPart[i+1:]
		}
		host, path = hostPart, rest
	default:
		host, path, _ = strings.Cut(s, "/")
		if i := strings.LastIndex(host, "@"); i >= 0 {
			host = host[i+1:]
		}
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	segments := strings.Split(path, "/")
	if host == "" || !validRepoToken(host, ".-") {
		return "", fmt.Errorf("repo %q: missing or invalid host (want host/owner/name, e.g. github.com/acme/api)", raw)
	}
	if len(segments) < 2 {
		return "", fmt.Errorf("repo %q: want host/owner/name, e.g. github.com/acme/api", raw)
	}
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." || !validRepoToken(seg, "._-") {
			return "", fmt.Errorf("repo %q: invalid path segment %q", raw, seg)
		}
	}
	return host + "/" + strings.Join(segments, "/"), nil
}

func validRepoToken(s, extra string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune(extra, r) {
			return false
		}
	}
	return s != ""
}

// NormalizeRepoList normalizes every entry, keeping order (the first is the
// primary repo) and rejecting duplicates after normalization.
func NormalizeRepoList(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		id, err := NormalizeRepo(v)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, fmt.Errorf("repo %q is listed twice", id)
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// RepoName is the last path segment of an identity, used in messages.
func RepoName(identity string) string {
	return identity[strings.LastIndex(identity, "/")+1:]
}

// RepoRef formats a deliverable inside a repo the way a shared Dossier records
// it: identity plus repo-relative path, never a machine-local absolute path.
func RepoRef(identity, relPath string) string {
	return identity + ":" + filepath.ToSlash(relPath)
}

// RepoLocation is the outcome of resolving one identity on this machine.
type RepoLocation struct {
	// Path is the checkout's top-level directory, or "" when unresolved.
	Path string
	// Via says how it was found: "session", "scan" or "manual" (from the learned
	// map, or a scan that just ran).
	Via string
	// Candidates lists several matching checkouts found by a scan. Dossier does
	// not guess between them; Path stays empty.
	Candidates []string
	// Stale is a learned path that no longer holds this repo and was dropped.
	Stale string
}

// RepoLocator resolves repo identities to local checkouts on this machine. The
// learned map it keeps is machine-local and never synced (B13).
type RepoLocator interface {
	// Identify returns the normalized origin identity and top-level directory of
	// the git repository containing dir; ok is false when dir is not inside a
	// repository with a usable origin remote.
	Identify(dir string) (identity, root string, ok bool)
	// Locate finds a local checkout for identity: the learned map first
	// (verified), then the configured search roots.
	Locate(identity string) (RepoLocation, error)
	// Remember records path as identity's checkout on this machine.
	Remember(identity, path, via string) error
}

// ResolvedRepo is one of a Dossier's repos with its local path, if any.
type ResolvedRepo struct {
	Identity string `json:"identity"`
	Path     string `json:"path,omitempty"`
	Via      string `json:"via,omitempty"`
}

// LaunchTarget is where an agent for a Dossier should start and what it should
// be told about the Dossier's repos.
type LaunchTarget struct {
	DossierDir string         `json:"dossier_dir"`
	WorkDir    string         `json:"work_dir"`
	Repos      []ResolvedRepo `json:"repos,omitempty"`
}

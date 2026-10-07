// Package repos resolves repo identities to local checkouts on this machine
// (ADR 0015). It implements core.RepoLocator over go-git (no git CLI, B12) and a
// machine-local learned map at <home>/local/repo-paths.json, which never syncs.
package repos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"

	"dossier/internal/core"
)

// LocalDirName is the store's machine-local state directory. Team Sync excludes
// it (internal/sync/gitignore.go).
const LocalDirName = "local"

const mapFileName = "repo-paths.json"

type entry struct {
	Path      string    `json:"path"`
	LearnedAt time.Time `json:"learned_at"`
	Via       string    `json:"via"`
}

// Locator implements core.RepoLocator.
type Locator struct {
	dossierHome string
	roots       []string
	now         func() time.Time
}

// New returns a Locator for the store at dossierHome that searches roots (from
// config.yaml repo_roots; "~/" is expanded) when the learned map has no entry.
func New(dossierHome string, roots []string) *Locator {
	return &Locator{dossierHome: dossierHome, roots: roots, now: time.Now}
}

var _ core.RepoLocator = (*Locator)(nil)

// Identify returns the origin identity and top-level directory of the git
// checkout containing dir. Worktrees resolve through their common .git.
func (l *Locator) Identify(dir string) (string, string, bool) {
	if dir == "" {
		return "", "", false
	}
	abs, err := filepath.Abs(expandTilde(dir))
	if err != nil {
		return "", "", false
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", "", false
	}
	repo, err := git.PlainOpenWithOptions(abs, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil {
		return "", "", false
	}
	remote, err := repo.Remote("origin")
	if err != nil || len(remote.Config().URLs) == 0 {
		return "", "", false
	}
	identity, err := core.NormalizeRepo(remote.Config().URLs[0])
	if err != nil {
		return "", "", false
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", "", false
	}
	return identity, wt.Filesystem.Root(), true
}

// Locate resolves identity: a learned path that still holds it, else a scan
// of the search roots one level deep. A single scan hit is remembered; several
// are returned as candidates without guessing.
func (l *Locator) Locate(identity string) (core.RepoLocation, error) {
	m, err := l.load()
	if err != nil {
		return core.RepoLocation{}, err
	}
	var loc core.RepoLocation
	if e, ok := m[identity]; ok {
		if got, _, ok := l.Identify(e.Path); ok && got == identity {
			return core.RepoLocation{Path: e.Path, Via: e.Via}, nil
		}
		loc.Stale = e.Path
		delete(m, identity)
		if err := l.save(m); err != nil {
			return loc, err
		}
	}
	var hits []string
	for _, root := range l.roots {
		entries, err := os.ReadDir(expandTilde(root))
		if err != nil {
			continue // a missing root is not an error; other roots may hold it
		}
		for _, de := range entries {
			if !de.IsDir() {
				continue
			}
			dir := filepath.Join(expandTilde(root), de.Name())
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				continue
			}
			if got, top, ok := l.Identify(dir); ok && got == identity {
				hits = append(hits, top)
			}
		}
	}
	sort.Strings(hits)
	switch len(hits) {
	case 0:
	case 1:
		loc.Path, loc.Via = hits[0], "scan"
		if err := l.Remember(identity, hits[0], "scan"); err != nil {
			return loc, err
		}
	default:
		loc.Candidates = hits
	}
	return loc, nil
}

// Remember records path as identity's checkout on this machine.
func (l *Locator) Remember(identity, path, via string) error {
	m, err := l.load()
	if err != nil {
		return err
	}
	if e, ok := m[identity]; ok && e.Path == path && e.Via == via {
		return nil
	}
	m[identity] = entry{Path: path, LearnedAt: l.now().UTC(), Via: via}
	return l.save(m)
}

func (l *Locator) mapPath() string {
	return filepath.Join(l.dossierHome, LocalDirName, mapFileName)
}

func (l *Locator) load() (map[string]entry, error) {
	m := map[string]entry{}
	data, err := os.ReadFile(l.mapPath())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s is corrupt (%v); delete it and Dossier will re-learn repo locations", l.mapPath(), err)
	}
	return m, nil
}

// save writes the map atomically. Two processes learning at once can drop one
// entry; it is a cache and is re-learned on the next bind or scan.
func (l *Locator) save(m map[string]entry) error {
	if err := os.MkdirAll(filepath.Dir(l.mapPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.mapPath()), mapFileName+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), l.mapPath())
}

func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Git keeps tracking a file that was committed before an ignore rule covered
// it, so adding a rule does not stop an older store from republishing that
// file (config.yaml.bak, .DS_Store) on every sync. The helpers here make sync
// treat Dossier's managed machine-local set as binding for tracked files too:
// such a file is removed from the index while its bytes stay on disk (git rm
// --cached), and a pull never overwrites, deletes or plants it.
//
// Only the managed set (defaultGitignoreEntries) counts. A rule a user adds to
// the store's .gitignore keeps git's normal meaning, so it never silently
// untracks shared work. The set is compiled in, so it cannot fail open on an
// unreadable or edited .gitignore, and a negation there cannot override it.

// managedLocalMatcher reports whether a repo-relative path is in the managed
// machine-local set.
func managedLocalMatcher() func(path string) bool {
	var patterns []gitignore.Pattern
	for _, line := range defaultGitignoreEntries {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, gitignore.ParsePattern(line, nil))
		}
	}
	m := gitignore.NewMatcher(patterns)
	return func(path string) bool {
		return m.Match(strings.Split(filepath.ToSlash(path), "/"), false)
	}
}

// untrackIgnored removes every matching entry from the index and leaves the
// working file alone. The next commit records the removal; once that commit is
// pushed, the team repo's current tree stops carrying the file. Earlier
// commits still hold it.
func untrackIgnored(repo *git.Repository, ignored func(string) bool) error {
	idx, err := repo.Storer.Index()
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}
	kept := idx.Entries[:0]
	removed := false
	for _, e := range idx.Entries {
		if ignored(e.Name) {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return nil
	}
	idx.Entries = kept
	if err := repo.Storer.SetIndex(idx); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	return nil
}

// untrackedWarning explains a file sync stopped tracking.
func untrackedWarning(path string) string {
	return fmt.Sprintf("stopped syncing %s: it is machine-local, but an older Dossier build published it. It is removed from the local sync index and kept on this machine; the next successful push removes the current remote copy, and earlier copies remain in the team repo's history", path)
}

// preserveIgnoredLocal records the local state of every ignored path that
// differs between the trees from and to, and returns a func that puts that
// state back after a checkout to "to": a regular file gets its bytes back, a
// symlink is recreated as the same link (never written through), and a path
// that was absent is removed again so a pull cannot plant a teammate's copy.
func preserveIgnoredLocal(storeDir string, ignored func(string) bool, from, to *object.Tree) (func() error, error) {
	type saved struct {
		absent bool
		link   string // symlink target, when the path was a symlink
		data   []byte
		mode   os.FileMode
	}
	keep := map[string]saved{}
	for path := range changeNames(diffTree(from, to)) {
		if !ignored(path) {
			continue
		}
		full := filepath.Join(storeDir, filepath.FromSlash(path))
		info, err := os.Lstat(full)
		switch {
		case os.IsNotExist(err):
			keep[path] = saved{absent: true}
		case err != nil:
			return nil, fmt.Errorf("preserve %s: %w", path, err)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(full)
			if err != nil {
				return nil, fmt.Errorf("preserve %s: %w", path, err)
			}
			keep[path] = saved{link: target}
		case info.Mode().IsRegular():
			data, err := os.ReadFile(full)
			if err != nil {
				return nil, fmt.Errorf("preserve %s: %w", path, err)
			}
			keep[path] = saved{data: data, mode: info.Mode().Perm()}
		default:
			return nil, fmt.Errorf("preserve %s: machine-local path is a %s, not a file; move it aside and sync again", path, info.Mode().Type())
		}
	}
	return func() error {
		var failed []string
		for path, s := range keep {
			full := filepath.Join(storeDir, filepath.FromSlash(path))
			// Remove whatever the checkout left; Remove does not follow links.
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				failed = append(failed, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			if s.absent {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			if s.link != "" {
				if err := os.Symlink(s.link, full); err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", path, err))
				}
				continue
			}
			// O_EXCL refuses to open through a symlink placed at the path.
			f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, s.mode)
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			_, werr := f.Write(s.data)
			cerr := f.Close()
			if werr != nil || cerr != nil {
				failed = append(failed, fmt.Sprintf("%s: write: %v, close: %v", path, werr, cerr))
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("restore machine-local files after pull: %s", strings.Join(failed, "; "))
		}
		return nil
	}, nil
}

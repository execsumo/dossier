package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dossier/internal/core"
	"dossier/internal/store"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type treeDossier struct {
	id   string
	path string // top-level directory, slash-separated
	fm   core.Frontmatter
}

type dossierRenamePlan struct {
	id         string
	basePath   string
	localPath  string
	remotePath string
	targetPath string
}

func indexTreeDossiers(tree *object.Tree) map[string]treeDossier {
	out := map[string]treeDossier{}
	if tree == nil {
		return out
	}
	files := tree.Files()
	_ = files.ForEach(func(file *object.File) error {
		if !strings.HasSuffix(file.Name, "/dossier.md") || strings.Count(file.Name, "/") != 1 {
			return nil
		}
		content, err := file.Contents()
		if err != nil {
			return nil
		}
		fm, _, err := store.ParseDossierFile(content)
		if err != nil || fm.ID == "" {
			return nil
		}
		out[fm.ID] = treeDossier{id: fm.ID, path: strings.TrimSuffix(file.Name, "/dossier.md"), fm: *fm}
		return nil
	})
	return out
}

// dossierRenamePlans recognizes a directory rename by immutable dossier ID.
// Remote wins if both sides chose different new slugs.
func dossierRenamePlans(baseTree, localTree, remoteTree *object.Tree) map[string]dossierRenamePlan {
	base := indexTreeDossiers(baseTree)
	local := indexTreeDossiers(localTree)
	remote := indexTreeDossiers(remoteTree)
	plans := map[string]dossierRenamePlan{}
	for id, l := range local {
		r, ok := remote[id]
		if !ok || l.path == r.path {
			continue
		}
		b := base[id]
		target := l.path
		if r.path != "" && r.path != b.path {
			target = r.path
		}
		plans[id] = dossierRenamePlan{
			id: id, basePath: b.path, localPath: l.path, remotePath: r.path,
			targetPath: target,
		}
	}
	return plans
}

func renamePlanForPath(path string, plans map[string]dossierRenamePlan) (dossierRenamePlan, bool) {
	top := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		top = path[:i]
	}
	for _, plan := range plans {
		if top == plan.basePath || top == plan.localPath || top == plan.remotePath {
			return plan, true
		}
	}
	return dossierRenamePlan{}, false
}

func logicalRenamePath(path string, plans map[string]dossierRenamePlan) string {
	plan, ok := renamePlanForPath(path, plans)
	if !ok {
		return path
	}
	top := path
	rest := ""
	if i := strings.IndexByte(path, '/'); i >= 0 {
		top, rest = path[:i], path[i:]
	}
	if top == plan.basePath || top == plan.localPath || top == plan.remotePath {
		return plan.targetPath + rest
	}
	return path
}

func logicalChangeSources(paths map[string]struct{}, tree *object.Tree, plans map[string]dossierRenamePlan) map[string]string {
	out := map[string]string{}
	for path := range paths {
		logical := logicalRenamePath(path, plans)
		if _, exists := out[logical]; !exists {
			out[logical] = path
		}
		if _, err := tree.FindEntry(path); err == nil {
			out[logical] = path // prefer the side's extant path over a deletion
		}
	}
	return out
}

func rewriteRenamedDossier(content []byte, plan dossierRenamePlan) ([]byte, error) {
	fm, body, err := store.ParseDossierFile(string(content))
	if err != nil {
		return nil, err
	}
	if fm.ID != plan.id {
		return nil, fmt.Errorf("dossier ID %q does not match rename plan %q", fm.ID, plan.id)
	}
	fm.Slug = plan.targetPath
	formatted, err := store.FormatDossierFile(*fm, body)
	return []byte(formatted), err
}

func prepareWorkingTreeRenames(storeDir string, plans map[string]dossierRenamePlan) error {
	for _, plan := range plans {
		if plan.localPath == plan.targetPath {
			continue
		}
		src := filepath.Join(storeDir, filepath.FromSlash(plan.localPath))
		dst := filepath.Join(storeDir, filepath.FromSlash(plan.targetPath))
		if _, err := filepath.Abs(dst); err != nil {
			return err
		}
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("cannot reconcile dossier rename %s: destination %s already exists", plan.id, plan.targetPath)
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move local dossier %s to remote slug %s: %w", plan.localPath, plan.targetPath, err)
		}
	}
	return nil
}

func checkoutBlobTo(repo *git.Repository, tree *object.Tree, storeDir, sourcePath, destinationPath string, plan *dossierRenamePlan) error {
	content, err := blobContent(repo, tree, sourcePath)
	if err != nil {
		return err
	}
	if plan != nil && strings.HasSuffix(destinationPath, "/dossier.md") {
		content, err = rewriteRenamedDossier(content, *plan)
		if err != nil {
			return fmt.Errorf("rewrite renamed dossier: %w", err)
		}
	}
	full := filepath.Join(storeDir, filepath.FromSlash(destinationPath))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(full, content, 0644); err != nil {
		return err
	}
	return nil
}

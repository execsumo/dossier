// Package exportout is the adapter-side half of `dossier export` (ADR 0017).
//
// core.Service.Export assembles the document and touches no file. Everything
// that does I/O lives here, once, so the CLI and the MCP tool resolve the
// output path, refuse unsafe targets, write atomically and record the audit
// event identically: no export logic is forked into an adapter.
package exportout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"dossier/internal/core"
)

// Options is one export request from an adapter.
type Options struct {
	// ID is the Dossier slug or id.
	ID string
	// Output is "" for the default location, "-" for stdout (the caller prints
	// the returned document), or an explicit file path.
	Output string
	// Force allows overwriting an existing explicit Output.
	Force bool
	// Inline keeps the document in Result.Data (ExportResult.Markdown).
	Inline bool
	// Actor is recorded on the audit event.
	Actor string
}

// Run exports a Dossier, writes the file (unless Output is "-"), and records the
// `exported` audit event. It returns the service result with Output filled in
// and the document itself, which is dropped from Result.Data unless Inline.
func Run(ctx context.Context, svc *core.Service, o Options) (core.Result, string, error) {
	res, err := svc.Export(ctx, core.ExportReq{ID: o.ID})
	if err != nil {
		return core.Result{}, "", err
	}
	data, ok := res.Data.(core.ExportResult)
	if !ok {
		return core.Result{}, "", core.NewError(core.ErrInternal, fmt.Sprintf("export returned unexpected data %T", res.Data))
	}
	doc := data.Markdown

	audited := "-"
	if o.Output == "-" {
		data.Output = "-"
	} else {
		path, err := resolveTarget(o, data, homes(svc))
		if err != nil {
			return core.Result{}, "", err
		}
		written, err := writeDocument(path, o.Output != "", o.Force, doc)
		if err != nil {
			return core.Result{}, "", err
		}
		data.Output = written
		audited = filepath.Base(written)
	}

	if err := svc.RecordExport(ctx, core.RecordExportReq{Export: data, Output: audited, Actor: o.Actor}); err != nil {
		res.Warnings = append(res.Warnings, core.Warning(fmt.Sprintf(
			"The export was produced but its audit event could not be recorded: %v", err)))
	}

	if !o.Inline {
		data.Markdown = ""
	}
	res.Data = data
	return res, doc, nil
}

// homes lists the store roots an export must never be written under.
func homes(svc *core.Service) []string {
	var out []string
	if h := svc.DossierHome(); h != "" {
		out = append(out, h)
	}
	if h := os.Getenv("DOSSIER_HOME"); h != "" && h != svc.DossierHome() {
		out = append(out, h)
	}
	return out
}

// resolveTarget returns the absolute path to write: the explicit Output, or the
// default ~/Downloads/<slug>-export-<date>.md (home directory when there is no
// ~/Downloads). The default does not depend on the working directory.
func resolveTarget(o Options, data core.ExportResult, storeRoots []string) (string, error) {
	var path string
	if o.Output != "" {
		path = o.Output
		if path == "~" || strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", core.WrapError(core.ErrInternal, "cannot resolve ~ in the output path", err)
			}
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", core.WrapError(core.ErrInvalidFrontmatter, fmt.Sprintf("invalid output path %q", o.Output), err)
		}
		path = abs
	} else {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", core.WrapError(core.ErrInternal, "cannot determine the home directory for the default export location; pass an output path", err)
		}
		dir := home
		if info, err := os.Stat(filepath.Join(home, "Downloads")); err == nil && info.IsDir() {
			dir = filepath.Join(home, "Downloads")
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-export-%s.md", data.Slug, data.Date))
	}
	for _, root := range storeRoots {
		if insideDir(path, root) {
			return "", core.NewError(core.ErrInvalidFrontmatter, fmt.Sprintf(
				"refusing to write the export inside the Dossier store (%s): it would sync to the team and be mistaken for evidence. Choose a path outside it", root))
		}
	}
	return path, nil
}

// caseInsensitiveFS reports whether the platform's default filesystem ignores
// case. Erring towards rejection is safe: the guard only ever refuses a path.
var caseInsensitiveFS = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// insideDir reports whether path resolves to dir or somewhere beneath it,
// following symlinks through the nearest existing ancestor.
func insideDir(path, dir string) bool {
	p, d := resolveExisting(path), resolveExisting(dir)
	if caseInsensitiveFS {
		// Default macOS (APFS) and Windows filesystems ignore case, so
		// ~/.Dossier is the store too.
		p, d = strings.ToLower(p), strings.ToLower(d)
	}
	rel, err := filepath.Rel(d, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveExisting evaluates symlinks for the longest existing prefix of path and
// re-appends the remainder, so a not-yet-created file can still be compared.
func resolveExisting(path string) string {
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	rest := ""
	for cur := path; ; {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return path
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

var errExists = errors.New("destination exists")

// writeDocument writes doc to path and returns the path actually written. A
// default path that collides gets -2, -3, ... appended; an explicit path that
// exists is refused unless force. The bytes go to a temp file in the target
// directory first and are moved into place, so no partial file is ever left.
func writeDocument(path string, explicit, force bool, doc string) (string, error) {
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", core.NewError(core.ErrInvalidFrontmatter, fmt.Sprintf("output directory %s does not exist", dir))
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return "", core.NewError(core.ErrInvalidFrontmatter, fmt.Sprintf("output path %s is a directory; give a file path", path))
	}

	if explicit {
		err := atomicWrite(path, doc, !force)
		if errors.Is(err, errExists) {
			return "", core.NewError(core.ErrInvalidFrontmatter, fmt.Sprintf("%s already exists; pass --force (force: true) to overwrite it", path))
		}
		if err != nil {
			return "", core.WrapError(core.ErrInternal, fmt.Sprintf("failed to write %s", path), err)
		}
		return path, nil
	}

	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for n := 1; n < 10000; n++ {
		candidate := path
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		err := atomicWrite(candidate, doc, true)
		if errors.Is(err, errExists) {
			continue
		}
		if err != nil {
			return "", core.WrapError(core.ErrInternal, fmt.Sprintf("failed to write %s", candidate), err)
		}
		return candidate, nil
	}
	return "", core.NewError(core.ErrInternal, fmt.Sprintf("no free export file name beside %s", path))
}

// atomicWrite writes data to a temp file beside path and moves it into place.
// With noClobber the move fails with errExists rather than replace anything.
func atomicWrite(path, data string, noClobber bool) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dossier-export-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if !noClobber {
		return os.Rename(tmp.Name(), path)
	}
	// os.Link fails if path exists, which makes the no-clobber check atomic.
	if linkErr := os.Link(tmp.Name(), path); linkErr == nil {
		_ = os.Remove(tmp.Name())
		return nil
	} else if os.IsExist(linkErr) {
		err = errExists
		return err
	}
	// Filesystems without hard links: check, then rename.
	if _, statErr := os.Lstat(path); statErr == nil {
		err = errExists
		return err
	}
	return os.Rename(tmp.Name(), path)
}

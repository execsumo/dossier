package harness

import (
	"dossier/assets"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// nodeModuleRoots lists the node_modules directories worth searching for the
// packages the type-check needs. `npm root -g` first, since that is where a
// global `pi` install lands.
func nodeModuleRoots(t *testing.T) []string {
	t.Helper()
	var roots []string
	if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			roots = append(roots, p)
		}
	}
	roots = append(roots, "/usr/lib/node_modules", "/usr/local/lib/node_modules")
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, ".npm-global", "lib", "node_modules"))
	}
	return roots
}

// findNodePackage locates one package directory across the candidate roots.
func findNodePackage(t *testing.T, name string) string {
	t.Helper()
	for _, root := range nodeModuleRoots(t) {
		candidate := filepath.Join(root, filepath.FromSlash(name))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		// A globally installed package may vendor its own copy.
		nested := filepath.Join(root, "@earendil-works", "pi-coding-agent", "node_modules", filepath.FromSlash(name))
		if info, err := os.Stat(nested); err == nil && info.IsDir() {
			return nested
		}
	}
	return ""
}

// findTypesNode locates @types/node, which the extension needs for node:fs,
// node:os, node:path and `process`. Global installs rarely carry it, so this
// also accepts a copy vendored inside any nearby project.
func findTypesNode(t *testing.T) string {
	t.Helper()
	if p := findNodePackage(t, "@types/node"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	var found string
	for _, base := range []string{filepath.Join(home, "projects"), home} {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(base, e.Name(), "node_modules", "@types", "node")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				found = candidate
				break
			}
		}
		if found != "" {
			break
		}
	}
	return found
}

// The bundled Pi extension is embedded and byte-compared, but nothing in the
// build had ever asserted it was valid TypeScript against Pi's actual extension
// API — a breaking API change would have surfaced at runtime as a session that
// silently reports no id. This type-checks the embedded asset against whatever
// Pi is installed on this machine, which is exactly the machine where such a
// break matters. It skips (rather than fails) where the toolchain is absent, so
// it costs a CI runner without Node nothing.
func TestPiExtensionTypeChecksAgainstInstalledPi(t *testing.T) {
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("tsc not on PATH; skipping Pi extension type-check")
	}
	piPkg := findNodePackage(t, "@earendil-works")
	if piPkg == "" {
		t.Skip("Pi (@earendil-works/pi-coding-agent) not installed; skipping Pi extension type-check")
	}
	if _, err := os.Stat(filepath.Join(piPkg, "pi-coding-agent")); err != nil {
		t.Skip("Pi package directory incomplete; skipping Pi extension type-check")
	}
	typesNode := findTypesNode(t)
	if typesNode == "" {
		t.Skip("@types/node not found; skipping Pi extension type-check")
	}

	source, err := assets.FS.ReadFile("pi-extension.ts")
	if err != nil {
		t.Fatalf("failed to read the embedded Pi extension: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.ts"), source, 0o644); err != nil {
		t.Fatalf("failed to stage the extension: %v", err)
	}
	modules := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(filepath.Join(modules, "@types"), 0o755); err != nil {
		t.Fatalf("failed to create node_modules: %v", err)
	}
	if err := os.Symlink(piPkg, filepath.Join(modules, "@earendil-works")); err != nil {
		t.Fatalf("failed to link Pi: %v", err)
	}
	if err := os.Symlink(typesNode, filepath.Join(modules, "@types", "node")); err != nil {
		t.Fatalf("failed to link @types/node: %v", err)
	}

	tsconfig := `{"compilerOptions":{"target":"ES2022","module":"ESNext",` +
		`"moduleResolution":"bundler","strict":true,"noEmit":true,` +
		`"skipLibCheck":true,"allowImportingTsExtensions":true,"types":["node"]},` +
		`"include":["index.ts"]}`
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("failed to write tsconfig: %v", err)
	}

	out, err := exec.Command(tsc, "-p", filepath.Join(dir, "tsconfig.json")).CombinedOutput()
	if err != nil {
		t.Errorf("the bundled Pi extension does not type-check against the installed Pi:\n%s", out)
	}
}

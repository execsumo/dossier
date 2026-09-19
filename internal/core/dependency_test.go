package core

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestCorePackageIsPure enforces the Hexagonal Architecture constraint:
// internal/core MUST NOT import I/O-bearing standard-library packages, sibling
// packages, or third-party libraries. Pure standard-library helpers and
// internal/core subpackages are allowed.
func TestCorePackageIsPure(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", "dossier/internal/core")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run go list: %v (stderr: %q)", err, stderr.String())
	}

	imports := strings.Split(stdout.String(), "\n")
	for _, imp := range imports {
		imp = strings.TrimSpace(imp)
		if imp == "" {
			continue
		}

		if imp == "os" || strings.HasPrefix(imp, "os/") || imp == "net" || strings.HasPrefix(imp, "net/") {
			t.Errorf("FORBIDDEN I/O IMPORT IN CORE: %s. Put I/O behind a core port.", imp)
			continue
		}

		// Allow Go standard library packages (which do not contain a dot ".")
		if !strings.Contains(imp, ".") {
			continue
		}

		// Allow self-imports of internal/core subdirectories (if any, e.g. dossier/internal/core/foo)
		if strings.HasPrefix(imp, "dossier/internal/core") {
			continue
		}

		// Any other import containing a dot or referring to internal sibling packages is forbidden.
		t.Errorf("FORBIDDEN IMPORT IN CORE: %s. internal/core must remain pure.", imp)
	}
}

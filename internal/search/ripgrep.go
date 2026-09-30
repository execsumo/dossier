package search

import (
	"bufio"
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// RipgrepSearcher implements core.Searcher using the system 'rg' command.
type RipgrepSearcher struct {
	dossierHome string
}

// NewRipgrepSearcher instantiates a new RipgrepSearcher.
func NewRipgrepSearcher(dossierHome string) *RipgrepSearcher {
	return &RipgrepSearcher{
		dossierHome: dossierHome,
	}
}

// IsRipgrepAvailable returns true if the 'rg' command-line tool is found in the path.
func IsRipgrepAvailable() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

// Search executes 'rg' and parses the output matches into Hits.
func (r *RipgrepSearcher) Search(ctx context.Context, query string, scope core.SearchScope) ([]core.Hit, error) {
	if !IsRipgrepAvailable() {
		return nil, fmt.Errorf("ripgrep is not installed or not in PATH")
	}

	if query == "" {
		return nil, nil
	}

	// Determine target path for ripgrep command
	targetPath := r.dossierHome
	if scope.DossierID != "" {
		// Find dossier dir path
		paths, err := store.DossierDirs(r.dossierHome)
		if err != nil {
			return nil, err
		}
		found := false
		for _, dirPath := range paths {
			data, err := os.ReadFile(filepath.Join(dirPath, "dossier.md"))
			if err != nil {
				continue
			}
			fm, _, err := store.ParseDossierFile(string(data))
			if err == nil && fm.ID == scope.DossierID {
				targetPath = dirPath
				found = true
				break
			}
		}
		if !found {
			return nil, core.NewError(core.ErrNotFound, fmt.Sprintf("dossier %q not found", scope.DossierID))
		}
	}

	// Prepare glob patterns to exclude non-dossier files
	args := []string{
		"-n", "-i", "-H", "--no-heading", "--color", "never",
		"-g", "!context/**",
		"-g", "!sessions/**",
		"-g", "!.*",
		"-g", "!audit.log",
		"-g", "!*.lock",
		"-g", "!*.tmp.*",
		query,
		targetPath,
	}

	cmd := exec.CommandContext(ctx, "rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Exit code 1 means no matches found, which is not an error
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("ripgrep execution failed: %w (stderr: %s)", err, stderr.String())
	}

	// Map to cache parsed dossier details to avoid repeatedly reading the same dossier.md files
	dossierCache := make(map[string]*core.Frontmatter)

	var hits []core.Hit
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}

		filePath := parts[0]
		lineNumStr := parts[1]
		lineText := parts[2]

		lineNum, err := strconv.Atoi(lineNumStr)
		if err != nil {
			continue
		}

		// Traverse up to find the dossier root directory containing dossier.md
		dossierDir := filepath.Dir(filePath)
		if filepath.Base(dossierDir) == "artifacts" || filepath.Base(dossierDir) == "conflicts" {
			dossierDir = filepath.Dir(dossierDir)
		}

		dossierPath := filepath.Join(dossierDir, "dossier.md")
		fm, cached := dossierCache[dossierPath]
		if !cached {
			data, err := os.ReadFile(dossierPath)
			if err != nil {
				continue
			}
			fm, _, err = store.ParseDossierFile(string(data))
			if err != nil {
				continue
			}
			dossierCache[dossierPath] = fm
		}

		// Determine if hit is on the dossier body itself or an artifact
		var artifactID string
		title := fm.Name

		if strings.Contains(filePath, "/artifacts/") {
			// Read artifact file to get its ID and Title
			artData, err := os.ReadFile(filePath)
			if err == nil {
				artParts := strings.SplitN(string(artData), "---", 3)
				if len(artParts) >= 3 {
					var artMeta struct {
						ID    string `yaml:"id"`
						Title string `yaml:"title"`
					}
					if yaml.Unmarshal([]byte(artParts[1]), &artMeta) == nil {
						artifactID = artMeta.ID
						title = artMeta.Title
					}
				}
			}
		}

		hits = append(hits, core.Hit{
			DossierID:   fm.ID,
			DossierName: fm.Name,
			ArtifactID:  artifactID,
			Title:       title,
			Path:        filePath,
			Snippet:     strings.TrimSpace(lineText),
			LineNumber:  lineNum,
		})
	}

	return hits, nil
}

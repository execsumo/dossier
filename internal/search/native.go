package search

import (
	"bufio"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// NativeSearcher implements core.Searcher using pure-Go recursive scans.
type NativeSearcher struct {
	dossierHome string
}

// NewNativeSearcher instantiates a new NativeSearcher.
func NewNativeSearcher(dossierHome string) *NativeSearcher {
	return &NativeSearcher{
		dossierHome: dossierHome,
	}
}

// Search scans files for the query within the given scope.
func (n *NativeSearcher) Search(ctx context.Context, query string, scope core.SearchScope) ([]core.Hit, error) {
	if query == "" {
		return nil, nil
	}

	queryLower := strings.ToLower(query)
	var hits []core.Hit

	// 1. Identify target directories
	var dirsToSearch []string
	if scope.DossierID != "" {
		// Single dossier search scope
		// We scan all directories to find the one matching the ID
		paths, err := store.DossierDirs(n.dossierHome)
		if err != nil {
			return nil, err
		}
		for _, dirPath := range paths {
			data, err := os.ReadFile(filepath.Join(dirPath, "dossier.md"))
			if err != nil {
				continue
			}
			fm, _, err := store.ParseDossierFile(string(data))
			if err == nil && fm.ID == scope.DossierID {
				dirsToSearch = append(dirsToSearch, dirPath)
				break
			}
		}
	} else {
		// Global search scope
		paths, err := store.DossierDirs(n.dossierHome)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		dirsToSearch = append(dirsToSearch, paths...)
	}

	// 2. Perform search in each directory
	for _, dirPath := range dirsToSearch {
		// Parse dossier.md to get ID and Name
		dossierPath := filepath.Join(dirPath, "dossier.md")
		data, err := os.ReadFile(dossierPath)
		if err != nil {
			continue
		}

		fm, body, err := store.ParseDossierFile(string(data))
		if err != nil {
			continue
		}

		// Search dossier body line-by-line
		bodyScanner := bufio.NewScanner(strings.NewReader(body))
		lineNum := 0
		for bodyScanner.Scan() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			lineNum++
			lineText := bodyScanner.Text()
			if strings.Contains(strings.ToLower(lineText), queryLower) {
				hits = append(hits, core.Hit{
					DossierID:   fm.ID,
					DossierName: fm.Name,
					Title:       fm.Name,
					Path:        dossierPath,
					Snippet:     strings.TrimSpace(lineText),
					LineNumber:  lineNum,
				})
			}
		}

		// Search artifacts
		artifactsDir := filepath.Join(dirPath, "artifacts")
		artEntries, err := os.ReadDir(artifactsDir)
		if err != nil {
			continue
		}

		for _, artEntry := range artEntries {
			if artEntry.IsDir() || strings.HasPrefix(artEntry.Name(), ".") {
				continue
			}

			artPath := filepath.Join(artifactsDir, artEntry.Name())
			artData, err := os.ReadFile(artPath)
			if err != nil {
				continue
			}

			// Parse artifact using store helper
			// Wait, since some helper functions are unexported or public, we made them public in fsstore:
			// `store.ParseDossierFile` is public. But what about artifacts?
			// Let's check: in our edit, we made parseArtifactFile unexported (`parseArtifactFile`), so we can't call it from outside!
			// Ah! We can easily parse it manually here or expose parseArtifactFile in store.
			// Let's check how artifacts are parsed: it's a SplitN with "---" on 3 parts.
			parts := strings.SplitN(string(artData), "---", 3)
			if len(parts) < 3 {
				continue
			}

			// We unmarshal the title and metadata from parts[1]
			var artMeta struct {
				ID    string `yaml:"id"`
				Title string `yaml:"title"`
			}
			if err := yaml.Unmarshal([]byte(parts[1]), &artMeta); err != nil {
				continue
			}

			artBody := parts[2]

			// Check title match
			if strings.Contains(strings.ToLower(artMeta.Title), queryLower) {
				hits = append(hits, core.Hit{
					DossierID:   fm.ID,
					DossierName: fm.Name,
					ArtifactID:  artMeta.ID,
					Title:       artMeta.Title,
					Path:        artPath,
					Snippet:     fmt.Sprintf("Title match: %s", artMeta.Title),
					LineNumber:  1,
				})
			}

			// Search artifact body line-by-line
			artScanner := bufio.NewScanner(strings.NewReader(artBody))
			artLineNum := 0
			for artScanner.Scan() {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}
				artLineNum++
				lineText := artScanner.Text()
				if strings.Contains(strings.ToLower(lineText), queryLower) {
					hits = append(hits, core.Hit{
						DossierID:   fm.ID,
						DossierName: fm.Name,
						ArtifactID:  artMeta.ID,
						Title:       artMeta.Title,
						Path:        artPath,
						Snippet:     strings.TrimSpace(lineText),
						LineNumber:  artLineNum,
					})
				}
			}
		}
	}

	return hits, nil
}

package search

import (
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestSearch(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-search-test-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// Create a dossier with an artifact
	dossierDir := filepath.Join(tempHome, "pricing-refresh")
	if err := os.MkdirAll(filepath.Join(dossierDir, "artifacts"), 0755); err != nil {
		t.Fatalf("failed to create layout: %v", err)
	}

	fm := core.Frontmatter{
		ID:        "dos_search_1",
		Name:      "Pricing model refresh",
		Slug:      "pricing-refresh",
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
		Status:    core.StatusActive, Priority: core.PriorityHigh,
	}
	body := "# Pricing model refresh\n\n## Situation\nWe are looking into concurrent locking problems."

	serializedDos, err := store.FormatDossierFile(fm, body)
	if err != nil {
		t.Fatalf("serialize dossier failed: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(serializedDos), 0644)

	art := &core.Artifact{
		ID:            "art_search_1",
		DossierID:     "dos_search_1",
		Type:          core.ArtifactTypeTranscript,
		Title:         "Sales Feedback Transcript",
		ContentFormat: core.ContentFormatText,
		Content:       "Sales prefers usage-tier packages over flat-tier commitments.",
	}
	// We call formatArtifactFile but since it's unexported we just format it manually or write it
	serializedArt, _ := yamlMarshalArtifact(art)
	_ = os.WriteFile(filepath.Join(dossierDir, "artifacts", "art_search_1.txt"), []byte(serializedArt), 0644)

	// Test 1: Native Searcher
	nativeSearcher := NewNativeSearcher(tempHome)
	ctx := context.Background()

	// Global query for body text
	hits1, err := nativeSearcher.Search(ctx, "concurrent", core.SearchScope{})
	if err != nil {
		t.Fatalf("NativeSearcher.Search failed: %v", err)
	}
	if len(hits1) != 1 {
		t.Errorf("expected 1 hit, got %d", len(hits1))
	} else if hits1[0].DossierID != "dos_search_1" || hits1[0].ArtifactID != "" {
		t.Errorf("unexpected hit result: %+v", hits1[0])
	}

	// Global query for artifact text
	hits2, err := nativeSearcher.Search(ctx, "usage-tier", core.SearchScope{})
	if err != nil {
		t.Fatalf("NativeSearcher.Search failed: %v", err)
	}
	if len(hits2) != 1 {
		t.Errorf("expected 1 hit, got %d", len(hits2))
	} else if hits2[0].ArtifactID != "art_search_1" {
		t.Errorf("unexpected hit result: %+v", hits2[0])
	}

	// Test 2: Ripgrep Searcher (if available)
	if IsRipgrepAvailable() {
		rgSearcher := NewRipgrepSearcher(tempHome)
		hits3, err := rgSearcher.Search(ctx, "concurrent", core.SearchScope{})
		if err != nil {
			t.Fatalf("RipgrepSearcher.Search failed: %v", err)
		}
		if len(hits3) != 1 {
			t.Errorf("Ripgrep: expected 1 hit, got %d", len(hits3))
		}

		hits4, err := rgSearcher.Search(ctx, "usage-tier", core.SearchScope{})
		if err != nil {
			t.Fatalf("RipgrepSearcher.Search failed: %v", err)
		}
		if len(hits4) != 1 {
			t.Errorf("Ripgrep: expected 1 hit, got %d", len(hits4))
		}
	}
}

func yamlMarshalArtifact(a *core.Artifact) (string, error) {
	yamlBytes, err := yaml.Marshal(a)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{"---", string(yamlBytes), "---", a.Content}, "\n"), nil
}

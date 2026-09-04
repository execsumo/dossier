package cli

import (
	"bytes"
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLICommands(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-cli-test-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// Pre-populate with a dossier
	dossierDir := filepath.Join(tempHome, "pricing-model-refresh")
	if err := os.MkdirAll(dossierDir, 0755); err != nil {
		t.Fatalf("failed to create dossier dir: %v", err)
	}

	fm := core.Frontmatter{
		ID:        "dos_test123",
		Name:      "Pricing model refresh",
		Slug:      "pricing-model-refresh",
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
		Status:    core.StatusActive, Priority: core.PriorityHigh,
		NextAction: "Compare revised scenarios",
	}
	body := "# Pricing model refresh\n\n## Situation\nWorking draft."

	serialized, err := store.FormatDossierFile(fm, body)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(serialized), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Lock file is needed for writes
	if err := os.WriteFile(filepath.Join(dossierDir, ".lock"), []byte{}, 0644); err != nil {
		t.Fatalf("failed to write lock: %v", err)
	}

	// 1. Run wire to get service
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("failed to wire: %v", err)
	}

	// Test List
	res, err := svc.List(context.Background(), core.ListReq{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	items, ok := res.Data.([]core.ListItem)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Slug != "pricing-model-refresh" {
		t.Errorf("expected slug pricing-model-refresh, got %s", items[0].Slug)
	}

	// Test Recall
	recallRes, err := svc.Recall(context.Background(), core.RecallReq{ID: "dos_test123"})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	recallData, ok := recallRes.Data.(core.RecallResult)
	if !ok {
		t.Fatalf("unexpected type for RecallResult: %T", recallRes.Data)
	}
	if recallData.Frontmatter.ID != "dos_test123" {
		t.Errorf("expected ID dos_test123, got %s", recallData.Frontmatter.ID)
	}

	// Test Path
	pathRes, err := svc.Path(context.Background(), core.PathReq{ID: "dos_test123"})
	if err != nil {
		t.Fatalf("Path failed: %v", err)
	}
	pathStr, ok := pathRes.Data.(string)
	if !ok || !strings.HasSuffix(pathStr, "pricing-model-refresh") {
		t.Errorf("expected suffix pricing-model-refresh, got %q", pathStr)
	}

	// Test Archive
	archiveRes, err := svc.Archive(context.Background(), core.ArchiveReq{ID: "dos_test123"})
	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}
	if !archiveRes.OK {
		t.Errorf("expected Archive result to be OK")
	}

	// Verify archived
	recallRes2, err := svc.Recall(context.Background(), core.RecallReq{ID: "dos_test123"})
	if err != nil {
		t.Fatalf("failed to recall archived dossier: %v", err)
	}
	recallData2 := recallRes2.Data.(core.RecallResult)
	if recallData2.Frontmatter.Status != core.StatusDone {
		t.Errorf("expected status to be done, got %s", recallData2.Frontmatter.Status)
	}
}

func TestCLIRenameSlugMovesDirectoryAndDropsOldReference(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tempHome := t.TempDir()
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Init(context.Background(), core.InitReq{YesToAll: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(context.Background(), core.SaveReq{
		FrontmatterUpdates: map[string]any{"name": "Old Slug"},
	}); err != nil {
		t.Fatalf("create dossier: %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"rename", "old-slug", "clearer-slug", "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rename command: %v", err)
	}

	svc, err = wire(tempHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Recall(context.Background(), core.RecallReq{ID: "old-slug"}); err == nil {
		t.Fatal("old slug still resolves after rename")
	}
	recalled, err := svc.Recall(context.Background(), core.RecallReq{ID: "clearer-slug"})
	if err != nil {
		t.Fatalf("new slug no longer resolves: %v", err)
	}
	got := recalled.Data.(core.RecallResult)
	if got.Frontmatter.Slug != "clearer-slug" {
		t.Fatalf("renamed frontmatter = %+v", got.Frontmatter)
	}
	if _, err := os.Stat(filepath.Join(tempHome, "clearer-slug", "dossier.md")); err != nil {
		t.Fatalf("renamed dossier path missing: %v", err)
	}
}

func TestCLIRenameTitleKeepsSlugAndPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tempHome := t.TempDir()
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Init(context.Background(), core.InitReq{YesToAll: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(context.Background(), core.SaveReq{
		FrontmatterUpdates: map[string]any{"name": "Old title"},
	}); err != nil {
		t.Fatalf("create dossier: %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"rename", "old-title", "--title", "New title", "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("title rename command: %v", err)
	}

	svc, err = wire(tempHome)
	if err != nil {
		t.Fatal(err)
	}
	recalled, err := svc.Recall(context.Background(), core.RecallReq{ID: "old-title"})
	if err != nil {
		t.Fatalf("title-renamed dossier no longer resolves: %v", err)
	}
	got := recalled.Data.(core.RecallResult)
	if got.Frontmatter.Name != "New title" || got.Frontmatter.Slug != "old-title" {
		t.Fatalf("renamed frontmatter = %+v", got.Frontmatter)
	}
	if filepath.Base(got.Path) != "old-title" {
		t.Fatalf("title rename changed path = %q", got.Path)
	}
}

func TestCLIPromoteDistilledFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tempHome := t.TempDir()
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("wire failed: %v", err)
	}
	if _, err := svc.Init(context.Background(), core.InitReq{YesToAll: true}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	raw := "The vendor changed the API contract.\nNeed to investigate migration impact."
	rawPath := filepath.Join(t.TempDir(), "spark.md")
	if err := os.WriteFile(rawPath, []byte(raw), 0600); err != nil {
		t.Fatalf("write raw capture: %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"promote", "Vendor API change", "--distilled-file", rawPath, "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("promote failed: %v", err)
	}

	svc, err = wire(tempHome)
	if err != nil {
		t.Fatalf("rewire failed: %v", err)
	}
	res, err := svc.List(context.Background(), core.ListReq{Status: "all"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	items := res.Data.([]core.ListItem)
	if len(items) != 1 {
		t.Fatalf("listed %d dossiers, want 1", len(items))
	}
	recall, err := svc.Recall(context.Background(), core.RecallReq{ID: items[0].ID})
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	dossier := recall.Data.(core.RecallResult)
	if dossier.Frontmatter.Status != core.StatusSpark || dossier.Frontmatter.Priority != core.PriorityMedium {
		t.Fatalf("new dossier metadata = status %q, priority %q; want spark/medium", dossier.Frontmatter.Status, dossier.Frontmatter.Priority)
	}
	if dossier.DistilledState != raw {
		t.Fatalf("distilled state = %q, want raw capture %q", dossier.DistilledState, raw)
	}
}

// TestCLIOutputFormat prints and verifies printed output strings
func TestCLIOutputFormat(t *testing.T) {
	// Simple validation of printJSON
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJSON(map[string]string{"foo": "bar"})

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	output := strings.TrimSpace(buf.String())
	expected := "{\n  \"foo\": \"bar\"\n}"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestCLIMilestone3(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-cli-m3-test-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// Initialize the store directories (which also writes context/library.md)
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("failed to wire: %v", err)
	}

	_, err = svc.Init(context.Background(), core.InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	// 1. Create a dossier using svc.Save
	saveReq := core.SaveReq{
		DistilledStateMarkdown: "# Product Specifications\n\nWe need to build a single Go binary.",
		FrontmatterUpdates: map[string]any{
			"name":        "Chainlink core engine",
			"status":      "active",
			"priority":    "high",
			"next_action": "Implement the MCP server",
		},
	}
	res, err := svc.Save(context.Background(), saveReq)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	// Let's list dossiers to get the actual ID
	listRes, err := svc.List(context.Background(), core.ListReq{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	items := listRes.Data.([]core.ListItem)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	realID := items[0].ID

	// Write an artifact to the dossier
	art := core.Artifact{
		ID:            "art_test_m3",
		DossierID:     realID,
		Type:          core.ArtifactTypeSourceSnapshot,
		Title:         "System design requirements document",
		ContentFormat: core.ContentFormatText,
		Content:       "Make sure it compiles into a single binary called dossier.",
	}
	// Save the dossier again with this artifact
	saveReq2 := core.SaveReq{
		ID:           realID,
		BaseRevision: res.Data.(core.Revision),
		Artifacts:    []core.Artifact{art},
	}
	_, err = svc.Save(context.Background(), saveReq2)
	if err != nil {
		t.Fatalf("Save with artifact failed: %v", err)
	}

	// 2. Perform global search
	searchRes, err := svc.Search(context.Background(), core.SearchReq{
		Query: "single",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	hits := searchRes.Data.([]core.Hit)
	if len(hits) != 2 {
		t.Errorf("expected 2 hits (dossier body and artifact), got %d", len(hits))
	}

	// 3. Perform scoped search to this dossier
	scopedRes, err := svc.Search(context.Background(), core.SearchReq{
		Query: "single",
		Scope: core.SearchScope{DossierID: realID},
	})
	if err != nil {
		t.Fatalf("Scoped search failed: %v", err)
	}
	scopedHits := scopedRes.Data.([]core.Hit)
	if len(scopedHits) != 2 {
		t.Errorf("expected 2 scoped hits, got %d", len(scopedHits))
	}

	// 4. Perform scoped search to a different/non-existent dossier
	_, err = svc.Search(context.Background(), core.SearchReq{
		Query: "single",
		Scope: core.SearchScope{DossierID: "dos_nonexistent"},
	})
	if err == nil {
		t.Errorf("expected error for nonexistent dossier scope, got nil")
	}

	// 5. Run context refresh
	refreshRes, err := svc.ContextRefresh(context.Background())
	if err != nil {
		t.Fatalf("ContextRefresh failed: %v", err)
	}
	if !refreshRes.OK {
		t.Fatalf("ContextRefresh returned not OK")
	}

	// Read generated library.md
	libBytes, err := os.ReadFile(filepath.Join(tempHome, "context", "library.md"))
	if err != nil {
		t.Fatalf("failed to read library.md: %v", err)
	}
	libContent := string(libBytes)

	if !strings.Contains(libContent, "# Dossier Library") {
		t.Errorf("expected # Dossier Library header in library.md, got:\n%s", libContent)
	}
	if !strings.Contains(libContent, "Chainlink core engine") {
		t.Errorf("expected 'Chainlink core engine' in library.md, got:\n%s", libContent)
	}
	if !strings.Contains(libContent, "Implement the MCP server") {
		t.Errorf("expected next action 'Implement the MCP server' in library.md, got:\n%s", libContent)
	}
}

func TestCLIMilestone5(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-cli-m5-test-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("failed to wire: %v", err)
	}

	_, err = svc.Init(context.Background(), core.InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	// 1. Promote a new dossier
	promRes, err := svc.Promote(context.Background(), core.PromoteReq{
		Name:                   "Pricing restructure project",
		DistilledStateMarkdown: "# Restructure",
		Content:                "Initial sales requirements transcript.",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}
	if !promRes.OK {
		t.Fatalf("expected Promote result to be OK")
	}
	dossierID := promRes.Data.(string)

	// 2. Try promoting again with same/similar name - should fail with AmbiguousTarget error
	_, err = svc.Promote(context.Background(), core.PromoteReq{
		Name:                   "Pricing restructure project",
		DistilledStateMarkdown: "# Duplicate",
		Force:                  false,
	})
	if err == nil {
		t.Fatalf("expected promote duplicate to fail with ambiguity error, got nil")
	}
	dErr, ok := err.(*core.DomainError)
	if !ok || dErr.Code != core.ErrAmbiguousTarget {
		t.Fatalf("expected ErrAmbiguousTarget error, got: %v", err)
	}

	// 3. Promote with Force=true should succeed
	promRes2, err := svc.Promote(context.Background(), core.PromoteReq{
		Name:                   "Pricing restructure project",
		DistilledStateMarkdown: "# Forced Duplicate",
		Force:                  true,
	})
	if err != nil {
		t.Fatalf("Forced promote failed: %v", err)
	}
	if !promRes2.OK {
		t.Fatalf("expected forced promote to succeed")
	}

	// 4. Link without ID (ambiguity check)
	linkRes, err := svc.Link(context.Background(), core.LinkReq{
		Content: "sales packaging pricing restructure info",
	})
	if err == nil {
		t.Fatalf("expected link without ID to fail with ambiguity error, got nil")
	}
	dErr2, ok := err.(*core.DomainError)
	if !ok || dErr2.Code != core.ErrAmbiguousTarget {
		t.Fatalf("expected ErrAmbiguousTarget error, got: %v", err)
	}
	candidates := linkRes.Data.([]core.Suggestion)
	if len(candidates) == 0 {
		t.Fatalf("expected candidates list to be non-empty")
	}

	// 5. Link with ID (successful attach)
	linkRes2, err := svc.Link(context.Background(), core.LinkReq{
		ID:      dossierID,
		Content: "Here is more sales feedback content to link.",
		Title:   "sales_feedback.txt",
	})
	if err != nil {
		t.Fatalf("Link with ID failed: %v", err)
	}
	if !linkRes2.OK {
		t.Fatalf("expected link with ID to be OK")
	}

	// Recall and verify ID matches
	recallRes, err := svc.Recall(context.Background(), core.RecallReq{ID: dossierID})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	recallData := recallRes.Data.(core.RecallResult)
	if recallData.Frontmatter.ID != dossierID {
		t.Errorf("expected ID %s, got %s", dossierID, recallData.Frontmatter.ID)
	}
}

func TestCLIMilestone6(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-cli-m6-test-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("failed to wire: %v", err)
	}

	_, err = svc.Init(context.Background(), core.InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	// 1. Promote a dossier
	promRes, err := svc.Promote(context.Background(), core.PromoteReq{
		Name:                   "Active Session Dossier",
		DistilledStateMarkdown: "# Initial Distilled State",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}
	dossierID := promRes.Data.(string)

	// 2. Switch to this dossier to bind it to a session
	sessionID := "sess_test_123"
	switchRes, err := svc.Switch(context.Background(), core.SwitchReq{
		ID:        dossierID,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Switch failed: %v", err)
	}
	if !switchRes.OK {
		t.Fatalf("expected Switch result to be OK")
	}

	// 3. Verify Active binding
	activeRes, err := svc.Active(context.Background(), core.ActiveReq{SessionID: sessionID})
	if err != nil {
		t.Fatalf("Active failed: %v", err)
	}
	binding := activeRes.Data.(*core.SessionBinding)
	if binding.DossierID != dossierID {
		t.Errorf("expected bound DossierID to be %s, got %s", dossierID, binding.DossierID)
	}

	// 4. Run SessionStart hook and verify context payload injection
	contextPayload, err := svc.SessionStart(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}
	if !strings.Contains(contextPayload, "# Initial Distilled State") {
		t.Errorf("expected Distilled State in session-start context, got:\n%s", contextPayload)
	}
	if !strings.Contains(contextPayload, "Active Session Dossier") {
		t.Errorf("expected dossier name in session-start context, got:\n%s", contextPayload)
	}

	// 5. Run SessionEnd hook with new distilled state and transcript
	_, err = svc.SessionEnd(context.Background(), sessionID, "# Updated Distilled State", "This is the final transcript of the session.")
	if err != nil {
		t.Fatalf("SessionEnd failed: %v", err)
	}

	// 6. Verify distilled state updated on disk
	recallRes, err := svc.Recall(context.Background(), core.RecallReq{ID: dossierID})
	if err != nil {
		t.Fatalf("Recall after session end failed: %v", err)
	}
	recallData := recallRes.Data.(core.RecallResult)
	if recallData.DistilledState != "# Updated Distilled State" {
		t.Errorf("expected updated distilled state, got %q", recallData.DistilledState)
	}

	// Verify transcript was saved as an artifact
	artPath := filepath.Join(tempHome, "active-session-dossier", "artifacts")
	entries, err := os.ReadDir(artPath)
	if err != nil {
		t.Fatalf("failed to read artifacts dir: %v", err)
	}
	foundTranscript := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "art_") {
			foundTranscript = true
			break
		}
	}
	if !foundTranscript {
		t.Errorf("expected transcript artifact to be written in artifacts/")
	}
}

func TestCLIMilestone7(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-cli-m7-test-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	// Set home flag so CLI uses our temp directory
	dossierHomeFlag = tempHome

	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("failed to wire: %v", err)
	}

	_, err = svc.Init(context.Background(), core.InitReq{YesToAll: true})
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	// 1. Create a dossier
	_, err = svc.Save(context.Background(), core.SaveReq{
		DistilledStateMarkdown: "Initial distilled content",
		FrontmatterUpdates: map[string]any{
			"name":        "Target Dossier",
			"status":      "active",
			"next_action": "Initial action",
		},
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	dossierID := "target-dossier"

	// 2. Test status subcommand via Cobra
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"status", dossierID, "waiting", "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status cmd execution failed: %v", err)
	}

	// 3. Test next subcommand via Cobra
	cmd = NewRootCmd()
	cmd.SetArgs([]string{"next", dossierID, "Do something next", "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("next cmd execution failed: %v", err)
	}

	// 4. Test priority subcommand via Cobra
	cmd = NewRootCmd()
	cmd.SetArgs([]string{"priority", dossierID, "--priority", "high", "--due", "2026-07-01", "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("priority cmd execution failed: %v", err)
	}

	// Verify all updates are reflected in the store
	d, err := svc.Recall(context.Background(), core.RecallReq{ID: dossierID})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	recall := d.Data.(core.RecallResult)

	if recall.Frontmatter.Status != core.StatusDelegated {
		t.Errorf("expected status 'delegated', got %s", recall.Frontmatter.Status)
	}
	if recall.Frontmatter.NextAction != "Do something next" {
		t.Errorf("expected next_action 'Do something next', got %q", recall.Frontmatter.NextAction)
	}
	if recall.Frontmatter.Priority != core.PriorityHigh {
		t.Errorf("expected priority high, got %s", recall.Frontmatter.Priority)
	}
	if recall.Frontmatter.DueDate != "2026-07-01" {
		t.Errorf("expected due date '2026-07-01', got %q", recall.Frontmatter.DueDate)
	}
	// 6. Create a source dossier to test merge
	_, err = svc.Save(context.Background(), core.SaveReq{
		DistilledStateMarkdown: "Source distilled content",
		FrontmatterUpdates: map[string]any{
			"name":   "Source Dossier",
			"status": "waiting",
		},
	})
	if err != nil {
		t.Fatalf("failed to create source dossier: %v", err)
	}
	sourceID := "source-dossier"

	// 7. Run merge subcommand via Cobra
	cmd = NewRootCmd()
	cmd.SetArgs([]string{"merge", sourceID, dossierID, "--home", tempHome})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("merge cmd execution failed: %v", err)
	}

	// Verify merge results
	mergedD, err := svc.Recall(context.Background(), core.RecallReq{ID: dossierID})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	mergedRecall := mergedD.Data.(core.RecallResult)

	if !strings.Contains(mergedRecall.DistilledState, "Source distilled content") {
		t.Errorf("expected merged distilled state to contain source body")
	}

	// Verify source was archived
	srcD, err := svc.Recall(context.Background(), core.RecallReq{ID: sourceID})
	if err != nil {
		t.Fatalf("Recall of source failed: %v", err)
	}
	srcRecall := srcD.Data.(core.RecallResult)
	if srcRecall.Frontmatter.Status != core.StatusDone {
		t.Errorf("expected source status to be done, got %s", srcRecall.Frontmatter.Status)
	}
}

func TestCLIInstall(t *testing.T) {
	tempTargetDir := t.TempDir()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"install", "--dir", tempTargetDir, "-y"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install cmd execution failed: %v", err)
	}

	destPath := filepath.Join(tempTargetDir, "dossier")
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("expected installed binary at %s, but got error: %v", destPath, err)
	}
	if info.IsDir() {
		t.Fatalf("expected installed file to be a regular file, but it's a directory")
	}

	// Verify idempotency
	cmdIdempotent := NewRootCmd()
	cmdIdempotent.SetArgs([]string{"install", "--dir", tempTargetDir, "-y"})
	if err := cmdIdempotent.Execute(); err != nil {
		t.Fatalf("idempotent install execution failed: %v", err)
	}
}

func TestVersionCommand(t *testing.T) {
	orig := Version
	Version = "v9.9.9-test"
	defer func() { Version = orig }()

	// Both `dossier version` and `dossier --version` print the same line.
	for _, args := range [][]string{{"version"}, {"--version"}} {
		cmd := NewRootCmd()
		// NewRootCmd reads Version at construction time, so build after setting it.
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v execution failed: %v", args, err)
		}
		if got := strings.TrimSpace(out.String()); got != "dossier v9.9.9-test" {
			t.Errorf("%v: expected %q, got %q", args, "dossier v9.9.9-test", got)
		}
	}
}

func TestCLIUpdate(t *testing.T) {
	// Create a mock server
	expectedContent := "mock-binary-content-12345"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dossier-test" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(expectedContent))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "dossier-binary")

	// Set env overrides
	t.Setenv("DOSSIER_UPDATE_URL", srv.URL+"/dossier-test")
	t.Setenv("DOSSIER_UPDATE_TARGET", targetPath)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"update"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update execution failed: %v", err)
	}

	// Verify the target path has the expected content and is executable
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	if string(data) != expectedContent {
		t.Errorf("expected %q, got %q", expectedContent, string(data))
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	// Check permissions on Unix (should be executable)
	mode := info.Mode()
	if mode&0111 == 0 {
		t.Errorf("expected file to be executable, got mode: %v", mode)
	}
}

func TestCLIDoneAndStatuses(t *testing.T) {
	tempHome := t.TempDir()
	svc, err := wire(tempHome)
	if err != nil {
		t.Fatalf("wire failed: %v", err)
	}

	// Create new dossier - verify default status is spark
	res, err := svc.Save(context.Background(), core.SaveReq{
		DistilledStateMarkdown: "# Spark Dossier\nInitial thoughts.",
		FrontmatterUpdates: map[string]any{
			"name": "Idea Spark",
		},
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	recallRes, err := svc.Recall(context.Background(), core.RecallReq{ID: "idea-spark"})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	recall := recallRes.Data.(core.RecallResult)
	if recall.Frontmatter.Status != core.StatusSpark {
		t.Fatalf("expected default status spark, got %s", recall.Frontmatter.Status)
	}

	// Transition through statuses via CLI
	statuses := []string{"define", "delegated", "review", "blocked"}
	for _, st := range statuses {
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"status", "idea-spark", st, "--home", tempHome})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("failed to set status %s: %v", st, err)
		}
		r, err := svc.Recall(context.Background(), core.RecallReq{ID: "idea-spark"})
		if err != nil {
			t.Fatalf("Recall failed: %v", err)
		}
		if r.Data.(core.RecallResult).Frontmatter.Status != core.Status(st) {
			t.Fatalf("expected status %s, got %s", st, r.Data.(core.RecallResult).Frontmatter.Status)
		}
	}

	// Mark done via dossier done
	doneCmd := NewRootCmd()
	doneCmd.SetArgs([]string{"done", "idea-spark", "--home", tempHome})
	if err := doneCmd.Execute(); err != nil {
		t.Fatalf("dossier done failed: %v", err)
	}
	rDone, err := svc.Recall(context.Background(), core.RecallReq{ID: "idea-spark"})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if rDone.Data.(core.RecallResult).Frontmatter.Status != core.StatusDone {
		t.Fatalf("expected status done, got %s", rDone.Data.(core.RecallResult).Frontmatter.Status)
	}
	_ = res
}

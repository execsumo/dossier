package store

import (
	"bufio"
	"bytes"
	"dossier/assets"
	"dossier/internal/core"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FSStore implements core.Store port interface using the local filesystem.
type FSStore struct {
	dossierHome string
}

// NewFSStore instantiates a filesystem-backed store.
func NewFSStore(dossierHome string) *FSStore {
	return &FSStore{
		dossierHome: dossierHome,
	}
}

// Init creates storage directories, writes the guide, and generates the library context.
func (s *FSStore) Init() error {
	dirs := []string{
		s.dossierHome,
		filepath.Join(s.dossierHome, "context"),
		filepath.Join(s.dossierHome, "sessions"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	guideContent, err := assets.FS.ReadFile("guide.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded guide: %w", err)
	}
	guidePath := filepath.Join(s.dossierHome, "context", "guide.md")
	if err := os.WriteFile(guidePath, guideContent, 0644); err != nil {
		return fmt.Errorf("failed to write guide.md: %w", err)
	}

	instructionsContent, err := assets.FS.ReadFile("instructions.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded instructions: %w", err)
	}
	instructionsPath := filepath.Join(s.dossierHome, "context", "instructions.md")
	if err := os.WriteFile(instructionsPath, instructionsContent, 0644); err != nil {
		return fmt.Errorf("failed to write instructions.md: %w", err)
	}

	data := core.LibraryData{
		Harness: "CLI",
		Capabilities: map[string]bool{
			"MCP":               false,
			"SessionStartHook":  false,
			"SessionEndHook":    false,
			"PreCompactionHook": false,
			"TranscriptCapture": false,
		},
		Warnings:     []string{"No harness session active. Run from within a supported client harness for full integration."},
		OpenDossiers: []core.LibraryDossier{},
	}
	if err := s.WriteLibraryContext(data); err != nil {
		return fmt.Errorf("failed to write initial library: %w", err)
	}

	return nil
}

// List scans the store for Dossier frontmatters.
func (s *FSStore) List(statusFilter string) ([]core.Frontmatter, error) {
	entries, err := os.ReadDir(s.dossierHome)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var list []core.Frontmatter
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "context" || name == "sessions" || strings.HasPrefix(name, ".") {
			continue
		}

		dirPath := filepath.Join(s.dossierHome, name)
		dossierPath := filepath.Join(dirPath, "dossier.md")
		data, err := os.ReadFile(dossierPath)
		if err != nil {
			continue
		}

		if statusFilter != "all" && !bytes.Contains(data, []byte("status: "+statusFilter)) {
			continue
		}

		fm, _, err := ParseDossierFile(string(data))
		if err != nil {
			continue
		}

		if statusFilter == "all" || string(fm.Status) == statusFilter {
			list = append(list, *fm)
		}
	}
	return list, nil
}

// Read reads a Dossier and its current Revision.
func (s *FSStore) Read(slugOrID string) (*core.Dossier, core.Revision, error) {
	dossierDir, err := s.findDossierDir(slugOrID)
	if err != nil {
		return nil, "", err
	}

	dossierPath := filepath.Join(dossierDir, "dossier.md")
	data, err := os.ReadFile(dossierPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read dossier.md: %w", err)
	}

	fm, body, err := ParseDossierFile(string(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse dossier file: %w", err)
	}

	artifacts, _ := s.listArtifactsInternal(fm.ID, dossierDir)
	rev := core.CalculateRevision(*fm, body, artifacts)

	return &core.Dossier{
		Frontmatter:    *fm,
		DistilledState: core.DistilledState{Body: body},
	}, rev, nil
}

// ReadRevision retrieves a specific historical version of a Dossier by revision hash.
func (s *FSStore) ReadRevision(slugOrID string, rev core.Revision) (*core.Dossier, error) {
	dossierDir, err := s.findDossierDir(slugOrID)
	if err != nil {
		return nil, err
	}

	// First check if current dossier matches this revision
	dossierPath := filepath.Join(dossierDir, "dossier.md")
	if _, err := os.Stat(dossierPath); err == nil {
		data, err := os.ReadFile(dossierPath)
		if err == nil {
			currFM, currBody, err := ParseDossierFile(string(data))
			if err == nil {
				currentArtifacts, _ := s.listArtifactsInternal(currFM.ID, dossierDir)
				currRev := core.CalculateRevision(*currFM, currBody, currentArtifacts)
				if currRev == rev {
					return &core.Dossier{
						Frontmatter:    *currFM,
						DistilledState: core.DistilledState{Body: currBody},
					}, nil
				}
			}
		}
	}

	// Check history folder
	historyPath := filepath.Join(dossierDir, "history", fmt.Sprintf("%s.md", rev))
	if _, err := os.Stat(historyPath); err == nil {
		data, err := os.ReadFile(historyPath)
		if err != nil {
			return nil, err
		}
		currFM, currBody, err := ParseDossierFile(string(data))
		if err != nil {
			return nil, err
		}
		return &core.Dossier{
			Frontmatter:    *currFM,
			DistilledState: core.DistilledState{Body: currBody},
		}, nil
	}

	return nil, core.NewError(core.ErrNotFound, fmt.Sprintf("revision %s not found", rev))
}

// Write writes a Dossier atomically checking concurrency.
func (s *FSStore) Write(d *core.Dossier, base core.Revision) (core.Revision, error) {
	slug := d.Frontmatter.Slug
	if slug == "" {
		slug = GenerateSlug(d.Frontmatter.Name)
		d.Frontmatter.Slug = slug
	}
	id := d.Frontmatter.ID
	if id == "" {
		var err error
		id, err = GenerateID("dos_")
		if err != nil {
			return "", err
		}
		d.Frontmatter.ID = id
	}

	dossierDir := filepath.Join(s.dossierHome, slug)

	// If editing an existing dossier, locate its path by ID first to handle renamed slugs
	var existingDir string
	if base != "" {
		existingDir, _ = s.findDossierDir(id)
	}

	if existingDir != "" {
		dossierDir = existingDir
	} else {
		// New dossier, check for slug collision
		if _, err := os.Stat(dossierDir); err == nil {
			slug = SlugWithSuffix(slug, id)
			d.Frontmatter.Slug = slug
			dossierDir = filepath.Join(s.dossierHome, slug)
		}
	}

	if err := os.MkdirAll(dossierDir, 0755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dossierDir, "artifacts"), 0755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dossierDir, "conflicts"), 0755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dossierDir, "history"), 0755); err != nil {
		return "", err
	}

	// Acquire Lock
	lock, err := Lock(filepath.Join(dossierDir, ".lock"))
	if err != nil {
		return "", fmt.Errorf("failed to acquire dossier lock: %w", err)
	}
	defer lock.Unlock()

	dossierPath := filepath.Join(dossierDir, "dossier.md")
	var currentRevision core.Revision
	var currentArtifacts []core.Artifact

	if _, err := os.Stat(dossierPath); err == nil {
		data, err := os.ReadFile(dossierPath)
		if err != nil {
			return "", fmt.Errorf("failed to read existing dossier: %w", err)
		}
		currFM, currBody, err := ParseDossierFile(string(data))
		if err != nil {
			return "", fmt.Errorf("failed to parse existing dossier: %w", err)
		}

		currentArtifacts, _ = s.listArtifactsInternal(id, dossierDir)
		currentRevision = core.CalculateRevision(*currFM, currBody, currentArtifacts)

		if base != "" && currentRevision != base {
			return "", core.NewError(core.ErrConcurrentEdit, fmt.Sprintf("concurrency mismatch: base is %q but current is %q", base, currentRevision))
		}

		// Save current state to history/<revision>.md before overwriting
		historyDir := filepath.Join(dossierDir, "history")
		historyPath := filepath.Join(historyDir, fmt.Sprintf("%s.md", currentRevision))
		if err := os.WriteFile(historyPath, data, 0644); err != nil {
			return "", fmt.Errorf("failed to save history archive: %w", err)
		}
	}

	// Update dates
	d.Frontmatter.UpdatedAt = time.Now().Truncate(time.Second)
	if d.Frontmatter.CreatedAt.IsZero() {
		d.Frontmatter.CreatedAt = d.Frontmatter.UpdatedAt
	}
	if d.Frontmatter.LastTouchedAt.IsZero() {
		d.Frontmatter.LastTouchedAt = d.Frontmatter.UpdatedAt
	}

	if err := d.Frontmatter.Validate(); err != nil {
		return "", core.WrapError(core.ErrInvalidFrontmatter, "invalid frontmatter details", err)
	}

	// Write temp file
	tempFile, err := os.CreateTemp(dossierDir, "dossier.md.tmp.*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	serialized, err := FormatDossierFile(d.Frontmatter, d.DistilledState.Body)
	if err != nil {
		tempFile.Close()
		return "", err
	}

	if _, err := tempFile.WriteString(serialized); err != nil {
		tempFile.Close()
		return "", err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return "", err
	}
	tempFile.Close()

	if err := os.Chmod(tempName, 0444); err != nil {
		return "", fmt.Errorf("failed to set read-only permissions: %w", err)
	}

	if err := os.Rename(tempName, dossierPath); err != nil {
		return "", fmt.Errorf("failed to atomically rename temp file: %w", err)
	}

	newRevision := core.CalculateRevision(d.Frontmatter, d.DistilledState.Body, currentArtifacts)
	return newRevision, nil
}

// WriteArtifact stores a source artifact file atomically.
func (s *FSStore) WriteArtifact(dossierID string, a *core.Artifact) error {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return err
	}

	lock, err := Lock(filepath.Join(dossierDir, ".lock"))
	if err != nil {
		return fmt.Errorf("failed to acquire dossier lock: %w", err)
	}
	defer lock.Unlock()

	dossierPath := filepath.Join(dossierDir, "dossier.md")
	if data, err := os.ReadFile(dossierPath); err == nil {
		if currFM, currBody, parseErr := ParseDossierFile(string(data)); parseErr == nil {
			currentArtifacts, _ := s.listArtifactsInternal(currFM.ID, dossierDir)
			currentRevision := core.CalculateRevision(*currFM, currBody, currentArtifacts)
			historyDir := filepath.Join(dossierDir, "history")
			if err := os.MkdirAll(historyDir, 0755); err != nil {
				return err
			}
			historyPath := filepath.Join(historyDir, fmt.Sprintf("%s.md", currentRevision))
			if _, statErr := os.Stat(historyPath); os.IsNotExist(statErr) {
				if err := os.WriteFile(historyPath, data, 0644); err != nil {
					return fmt.Errorf("failed to save history archive before artifact write: %w", err)
				}
			}
		}
	}

	if a.ID == "" {
		a.ID, err = GenerateID("art_")
		if err != nil {
			return err
		}
	}
	a.DossierID = dossierID
	if a.CapturedAt.IsZero() {
		a.CapturedAt = time.Now()
	}
	if a.RefreshedAt.IsZero() {
		a.RefreshedAt = a.CapturedAt
	}
	a.SourceSizeBytes = int64(len(a.Content))

	if err := a.Validate(); err != nil {
		return core.WrapError(core.ErrInvalidFrontmatter, "invalid artifact details", err)
	}

	artifactsDir := filepath.Join(dossierDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return err
	}

	ext := "txt"
	if a.ContentFormat == core.ContentFormatMarkdown {
		ext = "md"
	} else if a.ContentFormat == core.ContentFormatJSON {
		ext = "json"
	}
	fileName := fmt.Sprintf("%s.%s", a.ID, ext)
	filePath := filepath.Join(artifactsDir, fileName)

	tempFile, err := os.CreateTemp(artifactsDir, fileName+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp artifact: %w", err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	serialized, err := formatArtifactFile(a)
	if err != nil {
		tempFile.Close()
		return err
	}

	if _, err := tempFile.WriteString(serialized); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}
	tempFile.Close()

	if err := os.Chmod(tempName, 0444); err != nil {
		return fmt.Errorf("failed to set read-only permissions: %w", err)
	}

	if err := os.Rename(tempName, filePath); err != nil {
		return fmt.Errorf("failed to rename temp artifact file: %w", err)
	}

	return nil
}

// ReadArtifact retrieves an artifact from store.
func (s *FSStore) ReadArtifact(dossierID string, artifactID string) (*core.Artifact, error) {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return nil, err
	}

	artifactsDir := filepath.Join(dossierDir, "artifacts")
	exts := []string{"md", "json", "txt"}
	for _, ext := range exts {
		path := filepath.Join(artifactsDir, fmt.Sprintf("%s.%s", artifactID, ext))
		if _, err := os.Stat(path); err == nil {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return parseArtifactFile(string(data))
		}
	}

	return nil, core.NewError(core.ErrNotFound, fmt.Sprintf("artifact %q not found", artifactID))
}

// ListArtifacts lists all artifacts associated with a Dossier.
func (s *FSStore) ListArtifacts(dossierID string) ([]core.Artifact, error) {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return nil, err
	}
	return s.listArtifactsInternal(dossierID, dossierDir)
}

func (s *FSStore) listArtifactsInternal(dossierID string, dossierDir string) ([]core.Artifact, error) {
	artifactsDir := filepath.Join(dossierDir, "artifacts")
	entries, err := os.ReadDir(artifactsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var list []core.Artifact
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		art, err := parseArtifactFrontmatterOnly(filepath.Join(artifactsDir, entry.Name()))
		if err != nil {
			continue
		}
		list = append(list, *art)
	}
	return list, nil
}

// SanitizeAuthorString converts an author name to a path-safe string.
func SanitizeAuthorString(author string) string {
	author = strings.ToLower(author)
	var sb strings.Builder
	lastWasDash := false
	for _, r := range author {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			lastWasDash = false
		} else {
			if !lastWasDash {
				sb.WriteRune('-')
				lastWasDash = true
			}
		}
	}
	res := strings.Trim(sb.String(), "-")
	if res == "" {
		return "unknown"
	}
	return res
}

// AppendAudit logs a JSONL event.
func (s *FSStore) AppendAudit(dossierID string, e core.AuditEvent) error {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return err
	}

	author := e.Author
	if author == "" {
		author = "unknown"
	}
	safeAuthor := SanitizeAuthorString(author)

	auditDir := filepath.Join(dossierDir, "audit")
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		return err
	}

	return AppendAuditLine(filepath.Join(auditDir, safeAuthor+".log"), e)
}

// ReadAuditLog reads the audit events log.
func (s *FSStore) ReadAuditLog(dossierID string) ([]core.AuditEvent, error) {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return nil, err
	}

	var allEntries []core.AuditEvent

	legacyEntries, err := ReadAuditEntries(filepath.Join(dossierDir, "audit.log"))
	if err == nil && len(legacyEntries) > 0 {
		allEntries = append(allEntries, legacyEntries...)
	}

	auditDir := filepath.Join(dossierDir, "audit")
	files, err := os.ReadDir(auditDir)
	if err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".log") {
				shardEntries, err := ReadAuditEntries(filepath.Join(auditDir, f.Name()))
				if err == nil {
					allEntries = append(allEntries, shardEntries...)
				}
			}
		}
	}

	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].TS.Before(allEntries[j].TS)
	})

	return allEntries, nil
}

// ValidateAuditShards checks shard filenames and readability.
func (s *FSStore) ValidateAuditShards(dossierID string) []string {
	var issues []string
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return []string{fmt.Sprintf("could not find dossier %s", dossierID)}
	}
	auditDir := filepath.Join(dossierDir, "audit")
	files, err := os.ReadDir(auditDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []string{fmt.Sprintf("could not read audit dir for %s", dossierID)}
	}

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".log") {
			name := strings.TrimSuffix(f.Name(), ".log")
			if SanitizeAuthorString(name) != name {
				issues = append(issues, fmt.Sprintf("Audit shard %s in dossier %s has malformed name", f.Name(), dossierID))
			}
			_, err := ReadAuditEntries(filepath.Join(auditDir, f.Name()))
			if err != nil {
				issues = append(issues, fmt.Sprintf("Audit shard %s in dossier %s is unparseable: %v", f.Name(), dossierID, err))
			}
		}
	}
	return issues
}

// EnsureAuditDir creates the audit directory for a dossier.
func (s *FSStore) EnsureAuditDir(dossierID string) error {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return err
	}
	auditDir := filepath.Join(dossierDir, "audit")
	return os.MkdirAll(auditDir, 0755)
}

// WriteSessionStash writes a session transcript snapshot to the per-dossier stash.
func (s *FSStore) WriteSessionStash(dossierID string, author string, sessionID string, content string) error {
	dossierDir, err := s.findDossierDir(dossierID)
	if err != nil {
		return err
	}
	safeAuthor := SanitizeAuthorString(author)
	stashDir := filepath.Join(dossierDir, "sessions", safeAuthor)
	if err := os.MkdirAll(stashDir, 0755); err != nil {
		return err
	}
	filePath := filepath.Join(stashDir, fmt.Sprintf("%s.md", sessionID))

	// overwrite-or-append per session id is fine. We will overwrite.
	return os.WriteFile(filePath, []byte(content), 0644)
}

// SaveSessionBinding saves session bindings.
func (s *FSStore) SaveSessionBinding(binding *core.SessionBinding) error {
	sessionsDir := filepath.Join(s.dossierHome, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return err
	}
	filePath := filepath.Join(sessionsDir, fmt.Sprintf("%s.json", binding.SessionBindingID))
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// GetSessionBinding retrieves session bindings.
func (s *FSStore) GetSessionBinding(sessionID string) (*core.SessionBinding, error) {
	filePath := filepath.Join(s.dossierHome, "sessions", fmt.Sprintf("%s.json", sessionID))
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, core.NewError(core.ErrNotFound, fmt.Sprintf("session %q not found", sessionID))
		}
		return nil, err
	}

	var binding core.SessionBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

// ClearSessionBinding deletes session bindings.
func (s *FSStore) ClearSessionBinding(sessionID string) error {
	filePath := filepath.Join(s.dossierHome, "sessions", fmt.Sprintf("%s.json", sessionID))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// WriteConflict logs conflict states.
func (s *FSStore) WriteConflict(conflict *core.Conflict) error {
	dossierDir, err := s.findDossierDir(conflict.DossierID)
	if err != nil {
		return err
	}

	if conflict.ID == "" {
		conflict.ID, err = GenerateID("conf_")
		if err != nil {
			return err
		}
	}

	conflictsDir := filepath.Join(dossierDir, "conflicts")
	if err := os.MkdirAll(conflictsDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(conflictsDir, fmt.Sprintf("%s.md", conflict.ID))
	serialized, err := formatConflictFile(conflict)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(serialized), 0644)
}

// ReadConflict retrieves conflict states.
func (s *FSStore) ReadConflict(conflictID string) (*core.Conflict, error) {
	entries, err := os.ReadDir(s.dossierHome)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "context" || name == "sessions" || strings.HasPrefix(name, ".") {
			continue
		}

		conflictPath := filepath.Join(s.dossierHome, name, "conflicts", fmt.Sprintf("%s.md", conflictID))
		if _, err := os.Stat(conflictPath); err == nil {
			data, err := os.ReadFile(conflictPath)
			if err != nil {
				return nil, err
			}
			return parseConflictFile(string(data))
		}
	}

	return nil, core.NewError(core.ErrNotFound, fmt.Sprintf("conflict %q not found", conflictID))
}

// ListConflicts lists active unresolved conflicts.
func (s *FSStore) ListConflicts() ([]core.Conflict, error) {
	entries, err := os.ReadDir(s.dossierHome)
	if err != nil {
		return nil, err
	}

	var list []core.Conflict
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "context" || name == "sessions" || strings.HasPrefix(name, ".") {
			continue
		}

		conflictsDir := filepath.Join(s.dossierHome, name, "conflicts")
		confEntries, err := os.ReadDir(conflictsDir)
		if err != nil {
			continue
		}

		for _, confEntry := range confEntries {
			if confEntry.IsDir() || strings.HasPrefix(confEntry.Name(), ".") {
				continue
			}

			filePath := filepath.Join(conflictsDir, confEntry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			c, err := parseConflictFile(string(data))
			if err != nil {
				continue
			}
			list = append(list, *c)
		}
	}

	return list, nil
}

// Private helper methods

func (s *FSStore) findDossierDir(slugOrID string) (string, error) {
	directPath := filepath.Join(s.dossierHome, slugOrID)
	if info, err := os.Stat(directPath); err == nil && info.IsDir() {
		if slugOrID != "context" && slugOrID != "sessions" {
			if _, err := os.Stat(filepath.Join(directPath, "dossier.md")); err == nil {
				return directPath, nil
			}
		}
	}

	entries, err := os.ReadDir(s.dossierHome)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "context" || name == "sessions" || strings.HasPrefix(name, ".") {
			continue
		}

		dirPath := filepath.Join(s.dossierHome, name)
		dossierPath := filepath.Join(dirPath, "dossier.md")
		data, err := os.ReadFile(dossierPath)
		if err != nil {
			continue
		}

		if !bytes.Contains(data, []byte("id: "+slugOrID)) && !bytes.Contains(data, []byte("slug: "+slugOrID)) {
			continue
		}

		fm, _, err := ParseDossierFile(string(data))
		if err != nil {
			continue
		}

		if fm.ID == slugOrID || fm.Slug == slugOrID {
			return dirPath, nil
		}
	}

	return "", core.NewError(core.ErrNotFound, fmt.Sprintf("dossier %q not found", slugOrID))
}

// Public Parsing helpers exported for store integration tests

func ParseDossierFile(content string) (*core.Frontmatter, string, error) {
	startIdx := strings.Index(content, "---")
	if startIdx == -1 {
		return nil, "", fmt.Errorf("missing starting frontmatter delimiter")
	}

	endIdx := strings.Index(content[startIdx+3:], "---")
	if endIdx == -1 {
		return nil, "", fmt.Errorf("missing ending frontmatter delimiter")
	}
	endIdx += startIdx + 3

	yamlContent := content[startIdx+3 : endIdx]
	body := content[endIdx+3:]

	var fm core.Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, "", err
	}
	return &fm, strings.TrimPrefix(body, "\n"), nil
}

func FormatDossierFile(fm core.Frontmatter, body string) (string, error) {
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("---\n%s---\n%s", string(yamlBytes), body), nil
}

func parseArtifactFile(content string) (*core.Artifact, error) {
	startIdx := strings.Index(content, "---")
	if startIdx == -1 {
		return nil, fmt.Errorf("missing starting artifact frontmatter delimiter")
	}

	endIdx := strings.Index(content[startIdx+3:], "---")
	if endIdx == -1 {
		return nil, fmt.Errorf("missing ending artifact frontmatter delimiter")
	}
	endIdx += startIdx + 3

	yamlContent := content[startIdx+3 : endIdx]
	body := content[endIdx+3:]

	var art core.Artifact
	if err := yaml.Unmarshal([]byte(yamlContent), &art); err != nil {
		return nil, err
	}
	art.Content = strings.TrimPrefix(body, "\n")
	return &art, nil
}

func parseArtifactFrontmatterOnly(filePath string) (*core.Artifact, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var fmLines []string
	dashCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "---") {
			dashCount++
			if dashCount == 2 {
				break
			}
			continue
		}
		if dashCount == 1 {
			fmLines = append(fmLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if dashCount < 2 {
		return nil, fmt.Errorf("missing artifact frontmatter delimiters")
	}

	var art core.Artifact
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &art); err != nil {
		return nil, err
	}
	return &art, nil
}

func formatArtifactFile(a *core.Artifact) (string, error) {
	yamlBytes, err := yaml.Marshal(a)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("---\n%s---\n%s", string(yamlBytes), a.Content), nil
}

func formatConflictFile(c *core.Conflict) (string, error) {
	yamlBytes, err := yaml.Marshal(c)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(yamlBytes)
	sb.WriteString("---\n")
	sb.WriteString("## Rejected proposal\n")
	sb.WriteString(c.RejectedBody)
	sb.WriteString("\n\n## Diff against current\n")
	sb.WriteString(c.DiffAgainstCurrent)
	return sb.String(), nil
}

func parseConflictFile(content string) (*core.Conflict, error) {
	startIdx := strings.Index(content, "---")
	if startIdx == -1 {
		return nil, fmt.Errorf("missing starting conflict delimiter")
	}

	endIdx := strings.Index(content[startIdx+3:], "---")
	if endIdx == -1 {
		return nil, fmt.Errorf("missing ending conflict delimiter")
	}
	endIdx += startIdx + 3

	yamlContent := content[startIdx+3 : endIdx]
	body := content[endIdx+3:]

	var c core.Conflict
	if err := yaml.Unmarshal([]byte(yamlContent), &c); err != nil {
		return nil, err
	}

	subparts := strings.Split(body, "## Diff against current")
	if len(subparts) > 1 {
		c.DiffAgainstCurrent = strings.TrimSpace(subparts[1])
		proposalPart := subparts[0]
		proposalPart = strings.TrimPrefix(proposalPart, "## Rejected proposal\n")
		c.RejectedBody = strings.TrimSpace(proposalPart)
	} else {
		c.RejectedBody = strings.TrimSpace(body)
	}

	return &c, nil
}

// WriteLibraryContext renders the context library and writes it to context/library.md.
func (s *FSStore) WriteLibraryContext(data core.LibraryData) error {
	tmplContent, err := assets.FS.ReadFile("library.tmpl.md")
	if err != nil {
		return fmt.Errorf("failed to read embedded library template: %w", err)
	}

	tmpl, err := template.New("library").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse library template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return fmt.Errorf("failed to execute library template: %w", err)
	}

	ctxDir := filepath.Join(s.dossierHome, "context")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return fmt.Errorf("failed to create context directory: %w", err)
	}

	libraryPath := filepath.Join(ctxDir, "library.md")
	if err := os.WriteFile(libraryPath, rendered.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write library.md: %w", err)
	}

	return nil
}

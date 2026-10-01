package store

import (
	"bufio"
	"bytes"
	"dossier/internal/core"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var inboxIDPattern = regexp.MustCompile(`^inbox_[a-zA-Z0-9_-]+$`)

func (s *FSStore) inboxDir(dossierID string) (string, error) {
	dir, err := s.findDossierDir(dossierID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "inbox"), nil
}

func (s *FSStore) CreateInbox(item *core.InboxItem) error {
	if item == nil {
		return fmt.Errorf("inbox item is required")
	}
	if item.ID == "" {
		id, err := GenerateID("inbox_")
		if err != nil {
			return err
		}
		item.ID = id
	}
	return s.WriteInbox(item)
}

func (s *FSStore) ListInbox(dossierID string) ([]core.InboxItem, error) {
	dir, err := s.inboxDir(dossierID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []core.InboxItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]core.InboxItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		item, err := readInboxFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read inbox item %s: %w", entry.Name(), err)
		}
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ReceivedAt.Equal(items[j].ReceivedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].ReceivedAt.Before(items[j].ReceivedAt)
	})
	return items, nil
}

func (s *FSStore) ValidateInbox(dossierID string) []string {
	dir, err := s.inboxDir(dossierID)
	if err != nil {
		return []string{fmt.Sprintf("Dossier %s inbox: %v", dossierID, err)}
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []string{fmt.Sprintf("Dossier %s inbox: %v", dossierID, err)}
	}
	var issues []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			issues = append(issues, fmt.Sprintf("Dossier %s inbox contains unexpected entry %q", dossierID, entry.Name()))
			continue
		}
		item, err := readInboxFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			issues = append(issues, fmt.Sprintf("Dossier %s inbox item %s: %v", dossierID, entry.Name(), err))
			continue
		}
		if entry.Name() != item.ID+".md" || item.DossierID != dossierID {
			issues = append(issues, fmt.Sprintf("Dossier %s inbox item %s has mismatched identity", dossierID, entry.Name()))
		}
	}
	return issues
}

func (s *FSStore) ReadInbox(dossierID, inboxID string) (*core.InboxItem, error) {
	if !inboxIDPattern.MatchString(inboxID) {
		return nil, fmt.Errorf("invalid inbox item id %q", inboxID)
	}
	dir, err := s.inboxDir(dossierID)
	if err != nil {
		return nil, err
	}
	return readInboxFile(filepath.Join(dir, inboxID+".md"))
}

func (s *FSStore) WriteInbox(item *core.InboxItem) error {
	if item == nil || !inboxIDPattern.MatchString(item.ID) {
		return fmt.Errorf("invalid inbox item id")
	}
	if item.State != core.InboxPending && item.State != core.InboxAbsorbed && item.State != core.InboxDismissed {
		return fmt.Errorf("invalid inbox state %q", item.State)
	}
	if item.DossierID == "" || item.Source.Kind == "" || item.Excerpt == "" || item.RoutedBy == "" || item.ReceivedAt.IsZero() {
		return fmt.Errorf("inbox item is missing required fields")
	}
	dir, err := s.inboxDir(item.DossierID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(item)
	if err != nil {
		return err
	}
	content := append([]byte("---\n"), data...)
	content = append(content, []byte("---\n")...)
	path := filepath.Join(dir, item.ID+".md")
	tmp, err := os.CreateTemp(dir, item.ID+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readInboxFile(path string) (*core.InboxItem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	first, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(first) != "---" {
		return nil, fmt.Errorf("inbox item must start with YAML frontmatter")
	}
	var metadata bytes.Buffer
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if strings.TrimSpace(line) == "---" {
			break
		}
		metadata.WriteString(line)
		if readErr == io.EOF {
			return nil, fmt.Errorf("inbox item has unterminated frontmatter")
		}
	}
	var item core.InboxItem
	decoder := yaml.NewDecoder(&metadata)
	decoder.KnownFields(true)
	if err := decoder.Decode(&item); err != nil {
		return nil, err
	}
	if !inboxIDPattern.MatchString(item.ID) || item.State != core.InboxPending && item.State != core.InboxAbsorbed && item.State != core.InboxDismissed {
		return nil, fmt.Errorf("invalid inbox metadata")
	}
	return &item, nil
}

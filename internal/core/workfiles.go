package core

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// WorkingFile is one file under a Dossier's files/ namespace: loose
// deliverables, scratch, and attachments. Path is relative to the Dossier
// directory (for example "files/deck.pptx"), the same form the Distilled State's
// ## Files index uses.
type WorkingFile struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// FileStore is an optional store capability: enumerating a Dossier's files/ and
// reading one file's bytes (used by export, ADR 0017).
type FileStore interface {
	ListWorkingFiles(dossierID string) ([]WorkingFile, error)
	// ReadWorkingFile returns the content of relPath (the WorkingFile.Path form,
	// "files/..."). Implementations must refuse any path that is not a regular
	// file inside the Dossier's files/ directory.
	ReadWorkingFile(dossierID, relPath string) ([]byte, error)
}

// IsTextContent reports whether data is text a reader can be handed inline:
// valid UTF-8 with no NUL byte in its first 8 KB (ADR 0017 §3).
func IsTextContent(data []byte) bool {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

var filesIndexEntryRE = regexp.MustCompile("^\\s*[-*]\\s+`([^`]+)`")

// ParseFilesIndex returns the paths listed under the Distilled State's ## Files
// section, normalized to the relative form WorkingFile.Path uses. Entries that
// point outside files/ (an absolute path to a deliverable saved elsewhere) are
// returned unchanged; callers decide whether they can verify them.
func ParseFilesIndex(body string) []string {
	var paths []string
	inSection, inFence := false, false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			inSection = strings.EqualFold(strings.TrimSpace(strings.TrimLeft(trimmed, "#")), "Files") && strings.HasPrefix(trimmed, "## ")
			continue
		}
		if !inSection {
			continue
		}
		if m := filesIndexEntryRE.FindStringSubmatch(line); m != nil {
			paths = append(paths, strings.TrimPrefix(strings.TrimSpace(m[1]), "./"))
		}
	}
	return paths
}

// filesIndexIssues compares the ## Files index with what is on disk. A listed
// path that no longer exists is a broken pointer (an issue, like a citation that
// does not resolve); a file on disk the index never mentions is only an advisory,
// since the index is an authoring judgment. Index entries outside files/ cannot
// be checked from here and are skipped.
func filesIndexIssues(body string, files []WorkingFile) (missing []string, unindexed []string) {
	indexed := ParseFilesIndex(body)

	onDisk := make(map[string]bool, len(files))
	for _, f := range files {
		onDisk[f.Path] = true
	}
	covers := func(entry, path string) bool {
		if entry == path {
			return true
		}
		return strings.HasSuffix(entry, "/") && strings.HasPrefix(path, entry)
	}

	for _, entry := range indexed {
		if !strings.HasPrefix(entry, "files/") {
			continue
		}
		found := onDisk[entry]
		if !found && strings.HasSuffix(entry, "/") {
			for _, f := range files {
				if covers(entry, f.Path) {
					found = true
					break
				}
			}
		}
		if !found {
			missing = append(missing, entry)
		}
	}

	for _, f := range files {
		listed := false
		for _, entry := range indexed {
			if covers(entry, f.Path) {
				listed = true
				break
			}
		}
		if !listed {
			unindexed = append(unindexed, f.Path)
		}
	}
	sort.Strings(missing)
	sort.Strings(unindexed)
	return missing, unindexed
}

// unindexedFilesAdvisory renders the advisory for files the ## Files index never
// mentions, naming a bounded sample so a large folder does not flood the report.
func unindexedFilesAdvisory(dossierID string, unindexed []string) string {
	shown, suffix := unindexed, ""
	if len(shown) > 5 {
		shown = shown[:5]
		suffix = fmt.Sprintf(" (+%d more)", len(unindexed)-5)
	}
	return fmt.Sprintf(
		"Dossier %s has %d file(s) in files/ not listed under ## Files in the Distilled State: %s%s. List them so the next session can find the work.",
		dossierID, len(unindexed), strings.Join(shown, ", "), suffix)
}

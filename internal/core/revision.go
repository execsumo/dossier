package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CanonicalFrontmatter serializes a Frontmatter struct into a deterministic, ordered key-value format.
func CanonicalFrontmatter(fm Frontmatter) string {
	var sb strings.Builder
	// Fields are ordered alphabetically to ensure absolute determinism.
	sb.WriteString(fmt.Sprintf("created_at: %s\n", fm.CreatedAt.UTC().Format(time.RFC3339)))
	if fm.DueDate != "" {
		sb.WriteString(fmt.Sprintf("due_date: %s\n", fm.DueDate))
	}
	sb.WriteString(fmt.Sprintf("id: %s\n", fm.ID))
	sb.WriteString(fmt.Sprintf("importance: %s\n", fm.Importance))
	if len(fm.Interfaces) > 0 {
		sb.WriteString("interfaces:\n")
		for _, interfaceName := range fm.Interfaces {
			sb.WriteString(fmt.Sprintf("  - %s\n", interfaceName))
		}
	}
	sb.WriteString(fmt.Sprintf("last_touched_at: %s\n", fm.LastTouchedAt.UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("name: %s\n", fm.Name))
	sb.WriteString(fmt.Sprintf("next_action: %s\n", fm.NextAction))

	sb.WriteString("open_questions:\n")
	// Questions are kept in declared order
	for _, q := range fm.OpenQuestions {
		sb.WriteString(fmt.Sprintf("  - %s\n", q))
	}

	sb.WriteString(fmt.Sprintf("slug: %s\n", fm.Slug))
	sb.WriteString(fmt.Sprintf("status: %s\n", fm.Status))
	if fm.TokenTarget > 0 {
		sb.WriteString(fmt.Sprintf("token_target: %d\n", fm.TokenTarget))
	}
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", fm.UpdatedAt.UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("urgency: %s\n", fm.Urgency))
	return sb.String()
}

// NormalizeNewlines converts CRLF to LF, trims trailing whitespace on each line, and ensures a single trailing LF.
func NormalizeNewlines(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var trimmedLines []string
	for _, line := range lines {
		trimmedLines = append(trimmedLines, strings.TrimRight(line, " \t"))
	}
	content := strings.Join(trimmedLines, "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return "\n"
	}
	return content + "\n"
}

// CanonicalArtifacts generates a sorted representation of the artifacts and their hashes.
func CanonicalArtifacts(artifacts []Artifact) string {
	var artLines []string
	for _, art := range artifacts {
		h := sha256.Sum256([]byte(art.Content))
		artLines = append(artLines, fmt.Sprintf("%s:%s", art.ID, hex.EncodeToString(h[:])))
	}
	sort.Strings(artLines)
	return strings.Join(artLines, "\n")
}

// CalculateRevision computes the canonical SHA-256 hash (truncated to 32 chars) prefixed with "rev_".
func CalculateRevision(fm Frontmatter, body string, artifacts []Artifact) Revision {
	var sb strings.Builder
	sb.WriteString(CanonicalFrontmatter(fm))
	sb.WriteString("\n---\n")
	sb.WriteString(NormalizeNewlines(body))
	sb.WriteString("\n---artifacts---\n")
	sb.WriteString(CanonicalArtifacts(artifacts))

	h := sha256.Sum256([]byte(sb.String()))
	hexHash := hex.EncodeToString(h[:])
	if len(hexHash) > 32 {
		hexHash = hexHash[:32]
	}
	return Revision("rev_" + hexHash)
}

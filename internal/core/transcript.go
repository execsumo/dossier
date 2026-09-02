package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Transcript roles mirror the semantic roles a harness trace carries. A
// tool_call is an intention and a tool_result is an observation: identical
// text in the two positions means different things, so the compiled view
// keeps them distinct instead of flattening both into prose.
const (
	TranscriptRoleUser       = "user"
	TranscriptRoleAssistant  = "assistant"
	TranscriptRoleThinking   = "thinking"
	TranscriptRoleToolCall   = "tool_call"
	TranscriptRoleToolResult = "tool_result"
	TranscriptRoleSystem     = "system"
	TranscriptRoleUnparsed   = "unparsed"
)

// TranscriptNode is one addressable unit of a compiled transcript: a semantic
// role, an optional label (the tool name), the harness id it came from, and
// the verbatim content lines.
type TranscriptNode struct {
	Role  string
	Label string
	Ref   string
	Lines []string
}

// transcriptRecord is the subset of a harness JSONL record the compiler reads.
// Unknown fields are ignored; unknown record types are counted, never dropped
// silently.
type transcriptRecord struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Message *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Content   json.RawMessage `json:"content"`
	Timestamp string          `json:"timestamp"`
}

// transcriptBlock is a single content block inside a message.
type transcriptBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// nonContentRecordTypes are harness bookkeeping records that carry no
// conversational content (UI mode toggles, session bridging, cost rollups).
// They are tallied into the compiled header rather than rendered, so the
// elision is visible and countable rather than silent.
var nonContentRecordTypes = map[string]bool{
	"mode":                  true,
	"permission-mode":       true,
	"bridge-session":        true,
	"atis-latch":            true,
	"last-prompt":           true,
	"cost-state":            true,
	"file-history-delta":    true,
	"file-history-snapshot": true,
	"queue-operation":       true,
}

// CompileTranscript lowers a raw harness JSONL trace into a role-tagged plain
// text view with stable physical line numbers.
//
// The compiled text is the Dossier "full view": every conversational node is
// preserved verbatim, so a [src:art_x#L10-L20] range resolves to a meaningful
// span (an assistant turn, a tool result) instead of landing mid-JSON. Line
// numbers are the artifact file's own physical lines, assigned once at capture
// and frozen by the read-only artifact file, which is what lets search hits
// and artifact fetches share one coordinate system.
//
// Input that is not JSONL is returned unchanged: a plain-text transcript is
// already a usable full view.
func CompileTranscript(raw string) (string, []Warning) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}

	rawLines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var (
		nodes       []TranscriptNode
		unparsed    int
		nonContent  = map[string]int{}
		recordCount int
	)

	for _, rawLine := range rawLines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}
		var rec transcriptRecord
		if err := json.Unmarshal([]byte(trimmed), &rec); err != nil || rec.Type == "" {
			unparsed++
			nodes = append(nodes, TranscriptNode{
				Role:  TranscriptRoleUnparsed,
				Lines: []string{rawLine},
			})
			continue
		}
		recordCount++
		if nonContentRecordTypes[rec.Type] {
			nonContent[rec.Type]++
			continue
		}
		nodes = append(nodes, recordNodes(rec)...)
	}

	// A trace with records but no recognizable JSONL envelope is almost
	// certainly a plain-text transcript. Pass it through untouched.
	if recordCount == 0 {
		return raw, nil
	}

	var warnings []Warning
	if unparsed > 0 {
		warnings = append(warnings, Warning(fmt.Sprintf(
			"Transcript compile: %d line(s) were not valid JSONL and were preserved verbatim as [unparsed] nodes.", unparsed)))
	}

	return renderTranscript(nodes, recordCount, nonContent), warnings
}

// recordNodes lowers one JSONL record into zero or more IR nodes.
func recordNodes(rec transcriptRecord) []TranscriptNode {
	content := rec.Content
	role := rec.Type
	if rec.Message != nil {
		content = rec.Message.Content
		if rec.Message.Role != "" {
			role = rec.Message.Role
		}
	}
	if len(content) == 0 {
		return nil
	}

	// Content is either a bare string or an array of typed blocks.
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []TranscriptNode{{Role: normalizeRole(role), Lines: splitLines(text)}}
	}

	var blocks []transcriptBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return []TranscriptNode{{Role: TranscriptRoleUnparsed, Lines: splitLines(string(content))}}
	}

	var nodes []TranscriptNode
	for _, b := range blocks {
		if n, ok := blockNode(b, role); ok {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// blockNode lowers a single content block into an IR node.
func blockNode(b transcriptBlock, parentRole string) (TranscriptNode, bool) {
	switch b.Type {
	case "text":
		if strings.TrimSpace(b.Text) == "" {
			return TranscriptNode{}, false
		}
		return TranscriptNode{Role: normalizeRole(parentRole), Lines: splitLines(b.Text)}, true

	case "thinking":
		if strings.TrimSpace(b.Thinking) == "" {
			return TranscriptNode{}, false
		}
		return TranscriptNode{Role: TranscriptRoleThinking, Lines: splitLines(b.Thinking)}, true

	case "tool_use":
		return TranscriptNode{
			Role:  TranscriptRoleToolCall,
			Label: b.Name,
			Ref:   b.ID,
			Lines: renderToolInput(b.Input),
		}, true

	case "tool_result":
		return TranscriptNode{
			Role:  TranscriptRoleToolResult,
			Ref:   b.ToolUseID,
			Lines: renderToolResult(b.Content),
		}, true
	}
	return TranscriptNode{}, false
}

// renderToolInput expands a tool call's arguments one key per line, keeping
// string values verbatim. Tool arguments are the exact inputs a later reader
// needs to verify a decision, so they are never abbreviated here.
func renderToolInput(input json.RawMessage) []string {
	if len(input) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return splitLines(string(input))
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		var s string
		if err := json.Unmarshal(fields[k], &s); err == nil {
			valueLines := splitLines(s)
			if len(valueLines) == 1 {
				lines = append(lines, fmt.Sprintf("%s: %s", k, valueLines[0]))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s:", k))
			lines = append(lines, valueLines...)
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", k, string(fields[k])))
	}
	return lines
}

// renderToolResult flattens a tool result, which the harness encodes either as
// a bare string or as a block array.
func renderToolResult(content json.RawMessage) []string {
	if len(content) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return splitLines(s)
	}
	var blocks []transcriptBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		var lines []string
		for _, b := range blocks {
			if b.Text != "" {
				lines = append(lines, splitLines(b.Text)...)
			}
		}
		if len(lines) > 0 {
			return lines
		}
	}
	return splitLines(string(content))
}

func normalizeRole(role string) string {
	switch role {
	case TranscriptRoleUser, TranscriptRoleAssistant, TranscriptRoleSystem:
		return role
	case "":
		return TranscriptRoleSystem
	default:
		return role
	}
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// renderTranscript lowers the IR to the final full view. Each node gets a
// role-tagged header so a citation can name what it is citing, and the header
// block states exactly what was elided.
func renderTranscript(nodes []TranscriptNode, recordCount int, nonContent map[string]int) string {
	var sb strings.Builder
	sb.WriteString("# Compiled Session Transcript\n")
	sb.WriteString(fmt.Sprintf("Records read: %d. Content nodes: %d.\n", recordCount, len(nodes)))

	if len(nonContent) > 0 {
		types := make([]string, 0, len(nonContent))
		total := 0
		for t, n := range nonContent {
			types = append(types, fmt.Sprintf("%s=%d", t, n))
			total += n
		}
		sort.Strings(types)
		sb.WriteString(fmt.Sprintf(
			"Harness bookkeeping records not rendered (no conversational content): %d [%s]. The raw trace is retained in the session stash.\n",
			total, strings.Join(types, " ")))
	}
	sb.WriteString("Cite spans from this file as [src:<artifact_id>#L<start>-L<end>].\n")

	for i, n := range nodes {
		header := fmt.Sprintf("\n## [%d] %s", i+1, n.Role)
		if n.Label != "" {
			header += " " + n.Label
		}
		if n.Ref != "" {
			header += fmt.Sprintf(" (%s)", n.Ref)
		}
		sb.WriteString(header + "\n")
		for _, line := range n.Lines {
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

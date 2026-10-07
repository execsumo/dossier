package core

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Export assembles one self-contained Markdown brief for a reader who does not
// run Dossier (ADR 0017). It is a pure function over store reads: it writes
// nothing. The adapter writes the file and then calls RecordExport, so the
// audit trail only ever names exports that actually left the system.

// ExportReq addresses the Dossier to export.
type ExportReq struct {
	ID string
}

// Export exclusion kinds.
const (
	ExportExcludedTranscript = "transcript"
	ExportExcludedBinaryFile = "binary_file"
	ExportExcludedExternal   = "external_reference"
	ExportExcludedMissing    = "missing_file"
	ExportExcludedUnreadable = "unreadable"
)

// ExportExclusion is one item deliberately left out of the document.
type ExportExclusion struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
	Title  string `json:"title,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Reason string `json:"reason"`
}

// ExportResult is the Data of Export. Markdown is the document; adapters drop
// it from machine output unless the caller asked for it inline.
type ExportResult struct {
	DossierID         string            `json:"dossier_id"`
	Slug              string            `json:"slug"`
	Name              string            `json:"name"`
	Revision          Revision          `json:"revision"`
	Date              string            `json:"date"`
	ArtifactsIncluded []string          `json:"artifacts_included"`
	FilesIncluded     []string          `json:"files_included"`
	Excluded          []ExportExclusion `json:"excluded"`
	TokenEstimate     int               `json:"token_estimate"`
	// Output is filled by the adapter: the written path, or "-" for stdout.
	Output   string `json:"output,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

// exportPreamble is the fixed reader preamble, ADR 0017 §2 verbatim. The two
// %s verbs are the export date and the Dossier's updated_at date.
const exportPreamble = `**About this document.** This is a working Dossier exported on %s (last updated %s): the
author's full working context on one outcome, broader than any formal write-up it contains.
If you are an AI assistant helping the reader: answer from this document; say which section or
supporting item an answer comes from; when the document does not cover something, say so
plainly rather than inferring. Citations of the form ` + "`[src:art_…]`" + ` refer to the supporting
items below by ID; items listed under "Not included" were deliberately left out.`

const exportDateLayout = "2006-01-02"

// exportSection is a rendered chunk of the document, labelled for the
// local-path scan so a warning can say where the path occurs.
type exportSection struct {
	label string
	text  string
}

// Export builds the brief for req.ID. Result.Data is an ExportResult.
func (s *Service) Export(ctx context.Context, req ExportReq) (Result, error) {
	d, rev, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}
	fm := d.Frontmatter
	body := d.DistilledState.Body
	now := s.clock.Now()
	exportDate := now.Format(exportDateLayout)
	updated := fm.UpdatedAt.Format(exportDateLayout)

	var (
		warnings   []Warning
		excluded   []ExportExclusion
		sections   []exportSection
		artifactID []string
		fileIDs    []string
		support    strings.Builder
	)

	// Artifacts.
	artifacts, err := s.store.ListArtifacts(fm.ID)
	if err != nil {
		return Result{}, WrapError(ErrInternal, fmt.Sprintf("failed to list artifacts for dossier %s", fm.ID), err)
	}
	included, transcripts := exportArtifactOrder(body, artifacts)
	for _, art := range transcripts {
		excluded = append(excluded, ExportExclusion{
			Kind: ExportExcludedTranscript, ID: art.ID, Title: art.Title,
			Reason: "raw session transcripts are never exported",
		})
	}
	for _, it := range included {
		full, err := s.store.ReadArtifact(fm.ID, it.art.ID)
		if err != nil {
			excluded = append(excluded, ExportExclusion{
				Kind: ExportExcludedUnreadable, ID: it.art.ID, Title: it.art.Title,
				Reason: fmt.Sprintf("could not be read: %v", err),
			})
			continue
		}
		// The listing decided inclusion; re-check what was actually read. A
		// second artifact file claiming another's ID must not smuggle a
		// transcript (or anything else) out under a snapshot's listing.
		if full.ID != it.art.ID || full.Type == ArtifactTypeTranscript || full.Type != it.art.Type {
			excluded = append(excluded, ExportExclusion{
				Kind: ExportExcludedUnreadable, ID: it.art.ID, Title: it.art.Title,
				Reason: "its stored ID or type does not match the evidence index (likely a duplicate artifact ID); inspect artifacts/",
			})
			continue
		}
		heading := "### " + oneLine(full.Title)
		if len(it.superseded) > 0 {
			heading += " (supersedes " + oneLine(strings.Join(it.superseded, ", ")) + ")"
		}
		meta := []string{"ID: " + oneLine(full.ID), "Type: " + string(full.Type), "Captured: " + full.CapturedAt.Format(exportDateLayout)}
		if !full.RefreshedAt.IsZero() && !full.RefreshedAt.Equal(full.CapturedAt) {
			meta = append(meta, "Refreshed: "+full.RefreshedAt.Format(exportDateLayout))
		}
		if full.Provenance.Origin != "" {
			meta = append(meta, "Origin: "+oneLine(full.Provenance.Origin))
		}
		if full.Provenance.URL != "" {
			meta = append(meta, "URL: "+oneLine(full.Provenance.URL))
		}
		chunk := exportItemText(heading, strings.Join(meta, " · "), full.Content)
		support.WriteString(chunk)
		sections = append(sections, exportSection{label: "supporting item " + full.ID, text: chunk})
		artifactID = append(artifactID, full.ID)
	}

	// Working files and ## Files references.
	var onDisk []WorkingFile
	fileStore, hasFiles := s.store.(FileStore)
	if hasFiles {
		onDisk, err = fileStore.ListWorkingFiles(fm.ID)
		if err != nil {
			return Result{}, WrapError(ErrInternal, fmt.Sprintf("failed to list working files for dossier %s", fm.ID), err)
		}
	}
	for _, f := range onDisk {
		if !strings.HasPrefix(f.Path, "files/") {
			continue
		}
		data, err := fileStore.ReadWorkingFile(fm.ID, f.Path)
		if err != nil {
			excluded = append(excluded, ExportExclusion{
				Kind: ExportExcludedUnreadable, Path: f.Path, Size: f.Size,
				Reason: fmt.Sprintf("could not be read: %v", err),
			})
			continue
		}
		if !IsTextContent(data) {
			excluded = append(excluded, ExportExclusion{
				Kind: ExportExcludedBinaryFile, Path: f.Path, Size: int64(len(data)),
				Reason: "not inlined; keep a Markdown version in files/ to include it",
			})
			continue
		}
		meta := []string{"Path: " + oneLine(f.Path), "Type: working file"}
		if !f.Modified.IsZero() {
			meta = append(meta, "Modified: "+f.Modified.Format(exportDateLayout))
		}
		chunk := exportItemText("### "+oneLine(f.Path), strings.Join(meta, " · "), string(data))
		support.WriteString(chunk)
		sections = append(sections, exportSection{label: f.Path, text: chunk})
		fileIDs = append(fileIDs, f.Path)
	}
	onDiskSet := make(map[string]bool, len(onDisk))
	for _, f := range onDisk {
		onDiskSet[f.Path] = true
	}
	seenRef := map[string]bool{}
	for _, entry := range ParseFilesIndex(body) {
		if seenRef[entry] {
			continue
		}
		seenRef[entry] = true
		if !strings.HasPrefix(entry, "files/") {
			excluded = append(excluded, ExportExclusion{
				Kind: ExportExcludedExternal, Path: entry,
				Reason: "points outside this Dossier; not resolved or inlined",
			})
			continue
		}
		if strings.HasSuffix(entry, "/") {
			continue
		}
		if hasFiles && !onDiskSet[entry] {
			excluded = append(excluded, ExportExclusion{
				Kind: ExportExcludedMissing, Path: entry,
				Reason: "listed under ## Files but not found in files/",
			})
		}
	}

	// Not included.
	var notIncluded strings.Builder
	if len(excluded) == 0 {
		notIncluded.WriteString("Nothing was left out.\n")
	}
	for _, e := range excluded {
		notIncluded.WriteString(exportExclusionLine(e) + "\n")
	}

	// Document.
	headerParts := []string{"Exported: " + exportDate, "Last updated: " + updated, "Status: " + string(fm.Status)}
	if fm.Lead != "" {
		headerParts = append(headerParts, "Lead: "+oneLine(fm.Lead))
	}
	if fm.NextAction != "" {
		headerParts = append(headerParts, "Next action: "+oneLine(fm.NextAction))
	}
	var doc strings.Builder
	doc.WriteString("# " + oneLine(fm.Name) + "\n\n")
	doc.WriteString(strings.Join(headerParts, " · ") + "\n\n")
	doc.WriteString(fmt.Sprintf(exportPreamble, exportDate, updated) + "\n\n")
	doc.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		doc.WriteString("\n")
	}
	doc.WriteString("\n## Supporting material\n\n")
	if support.Len() == 0 {
		doc.WriteString("None.\n")
	} else {
		doc.WriteString(support.String())
	}
	doc.WriteString("\n## Not included\n\n")
	doc.WriteString(notIncluded.String())
	markdown := doc.String()

	// Warnings. None rewrites or trims content.
	header := "# " + oneLine(fm.Name) + "\n" + strings.Join(headerParts, " · ")
	scan := append([]exportSection{{label: "the header", text: header}, {label: "the Distilled State", text: body}}, sections...)
	scan = append(scan, exportSection{label: "Not included", text: notIncluded.String()})
	warnings = append(warnings, localPathWarnings(scan)...)

	if conflicts, err := s.store.ListConflicts(); err != nil {
		warnings = append(warnings, Warning(fmt.Sprintf("Conflicts could not be listed, so unresolved conflicts were not checked: %v", err)))
	} else {
		var open []string
		for _, c := range conflicts {
			if c.DossierID == fm.ID && c.ResolvedAt == nil {
				open = append(open, c.ID)
			}
		}
		if len(open) > 0 {
			warnings = append(warnings, Warning(fmt.Sprintf(
				"%d unresolved conflict(s) on this Dossier (%s); the brief may not be settled.", len(open), strings.Join(open, ", "))))
		}
	}

	estimate := s.tok.Estimate(markdown)
	if limit := s.TokenLimit(); estimate > limit {
		warnings = append(warnings, Warning(fmt.Sprintf(
			"The export is about %d tokens, over the token_limit of %d. It was not trimmed; the reader's assistant may not fit it in one context.", estimate, limit)))
	}

	var nTranscript, nBinary, nExternal int
	for _, e := range excluded {
		switch e.Kind {
		case ExportExcludedTranscript:
			nTranscript++
		case ExportExcludedBinaryFile:
			nBinary++
		case ExportExcludedExternal:
			nExternal++
		}
	}
	if nTranscript+nBinary+nExternal > 0 {
		warnings = append(warnings, Warning(fmt.Sprintf(
			"%d transcripts, %d binary files, %d external file references not included.", nTranscript, nBinary, nExternal)))
	}
	for _, e := range excluded {
		switch e.Kind {
		case ExportExcludedMissing, ExportExcludedUnreadable:
			warnings = append(warnings, Warning(fmt.Sprintf("%s not included: %s", exportExclusionName(e), e.Reason)))
		}
	}

	return Result{
		OK: true,
		Data: ExportResult{
			DossierID:         fm.ID,
			Slug:              fm.Slug,
			Name:              fm.Name,
			Revision:          rev,
			Date:              exportDate,
			ArtifactsIncluded: nonNilStrings(artifactID),
			FilesIncluded:     nonNilStrings(fileIDs),
			Excluded:          nonNilExclusions(excluded),
			TokenEstimate:     estimate,
			Markdown:          markdown,
		},
		Warnings: warnings,
	}, nil
}

// RecordExportReq describes a successful export for the audit log.
type RecordExportReq struct {
	Actor  string
	Export ExportResult
	// Output is the written file's basename, or "-" for stdout. Never a full path.
	Output string
}

// RecordExport appends the `exported` audit event. Adapters call it after the
// document has been written (or sent to stdout); it changes no revision.
func (s *Service) RecordExport(ctx context.Context, req RecordExportReq) error {
	return s.store.AppendAudit(req.Export.DossierID, AuditEvent{
		TS:                s.clock.Now(),
		Event:             AuditEventExported,
		Actor:             NormalizeActor(req.Actor, s.cfg.Author),
		Author:            s.cfg.Author,
		DossierID:         req.Export.DossierID,
		Revision:          string(req.Export.Revision),
		ArtifactsIncluded: req.Export.ArtifactsIncluded,
		FilesIncluded:     req.Export.FilesIncluded,
		Output:            req.Output,
	})
}

type exportArtifact struct {
	art        Artifact
	superseded []string
}

// exportArtifactOrder splits artifacts into the ones to inline and the
// transcripts to exclude. Of each (type, title, provenance.url) group only the
// newest is kept (by refreshed_at, then captured_at), carrying the IDs it
// supersedes. Order: groups cited by the Distilled State in order of first
// citation (a group is cited if any member is), then uncited groups by
// captured_at.
func exportArtifactOrder(body string, artifacts []Artifact) (included []exportArtifact, transcripts []Artifact) {
	firstCite := map[string]int{}
	for i, m := range provenanceRefRE.FindAllStringSubmatch(body, -1) {
		if _, ok := firstCite[m[1]]; !ok {
			firstCite[m[1]] = i
		}
	}

	type group struct {
		members []Artifact
		cite    int // -1 when no member is cited
	}
	groups := map[string]*group{}
	var keys []string
	for _, art := range artifacts {
		if art.Type == ArtifactTypeTranscript {
			transcripts = append(transcripts, art)
			continue
		}
		// Only snapshots supersede one another; two decision records that happen
		// to share a title are distinct evidence.
		key := "id\x00" + art.ID
		if art.Type == ArtifactTypeSourceSnapshot || art.Type == ArtifactTypeFileSnapshot {
			key = string(art.Type) + "\x00" + art.Title + "\x00" + art.Provenance.URL
		}
		g, ok := groups[key]
		if !ok {
			g = &group{cite: -1}
			groups[key] = g
			keys = append(keys, key)
		}
		g.members = append(g.members, art)
		if idx, ok := firstCite[art.ID]; ok && (g.cite < 0 || idx < g.cite) {
			g.cite = idx
		}
	}

	newer := func(a, b Artifact) bool { // a is newer than b
		if !a.RefreshedAt.Equal(b.RefreshedAt) {
			return a.RefreshedAt.After(b.RefreshedAt)
		}
		if !a.CapturedAt.Equal(b.CapturedAt) {
			return a.CapturedAt.After(b.CapturedAt)
		}
		return a.ID > b.ID
	}
	type ranked struct {
		item exportArtifact
		cite int
	}
	var all []ranked
	for _, key := range keys {
		g := groups[key]
		sort.SliceStable(g.members, func(i, j int) bool { return newer(g.members[i], g.members[j]) })
		item := exportArtifact{art: g.members[0]}
		rest := append([]Artifact{}, g.members[1:]...)
		sort.SliceStable(rest, func(i, j int) bool { return rest[i].CapturedAt.Before(rest[j].CapturedAt) })
		for _, r := range rest {
			item.superseded = append(item.superseded, r.ID)
		}
		all = append(all, ranked{item: item, cite: g.cite})
	}
	sort.SliceStable(all, func(i, j int) bool {
		ci, cj := all[i].cite, all[j].cite
		switch {
		case ci >= 0 && cj >= 0:
			return ci < cj
		case ci >= 0:
			return true
		case cj >= 0:
			return false
		}
		a, b := all[i].item.art, all[j].item.art
		if !a.CapturedAt.Equal(b.CapturedAt) {
			return a.CapturedAt.Before(b.CapturedAt)
		}
		return a.ID < b.ID
	})
	for _, r := range all {
		included = append(included, r.item)
	}
	return included, transcripts
}

// exportItemText renders one supporting item: heading, metadata line, and the
// content in a fence longer than any backtick run inside it.
func exportItemText(heading, meta, content string) string {
	fence := exportFence(content)
	var b strings.Builder
	b.WriteString(heading + "\n\n" + meta + "\n\n" + fence + "\n" + content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(fence + "\n\n")
	return b.String()
}

// exportFence returns a backtick fence longer than any backtick run in content
// (minimum three), so the content can never close its own fence.
func exportFence(content string) string {
	longest, run := 0, 0
	for _, r := range content {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
			continue
		}
		run = 0
	}
	n := 3
	if longest >= n {
		n = longest + 1
	}
	return strings.Repeat("`", n)
}

func exportExclusionName(e ExportExclusion) string {
	switch {
	case e.ID != "" && e.Title != "":
		return fmt.Sprintf("%s (%s)", e.ID, e.Title)
	case e.ID != "":
		return e.ID
	}
	return e.Path
}

func exportExclusionLine(e ExportExclusion) string {
	// One physical line per entry, so a filename or error text cannot forge
	// document structure.
	name := oneLine(exportExclusionName(e))
	reason := oneLine(e.Reason)
	switch e.Kind {
	case ExportExcludedTranscript:
		return fmt.Sprintf("- %s: transcript. %s.", name, capitalize(reason))
	case ExportExcludedBinaryFile:
		return fmt.Sprintf("- %s: binary file (%s), %s.", name, humanBytes(e.Size), reason)
	}
	return fmt.Sprintf("- %s: %s.", name, capitalize(reason))
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// oneLine collapses a value into a single line so a title cannot break the
// heading or metadata structure.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// localPathRE finds absolute paths under a home directory (Unix, macOS,
// Windows), including inside file:// URLs. The leading group keeps it from
// matching the middle of a longer path or word.
var localPathRE = regexp.MustCompile("(?m)(?:^|[\\s(\\[\"'`=:,<]|file://[^/\\s]*)((?:/home/[^/\\s`\"'<>)\\]]+|/Users/[^/\\s`\"'<>)\\]]+)(?:/[^\\s`\"'<>)\\]]*)?|~/[^\\s`\"'<>)\\]]*|(?i:[a-z]:\\\\users\\\\)[^\\\\\\s]+(?:\\\\[^\\s`\"'<>)\\]]*)?)")

// localPathWarnings names each home-directory path once, with where it occurs.
// The content is never changed: rewording a constraint is the owner's call.
func localPathWarnings(sections []exportSection) []Warning {
	where := map[string][]string{}
	var order []string
	for _, sec := range sections {
		for _, m := range localPathRE.FindAllStringSubmatch(sec.text, -1) {
			p := strings.TrimRight(m[1], ".,;:!?")
			if p == "" {
				continue
			}
			locs, seen := where[p]
			if !seen {
				order = append(order, p)
			}
			dup := false
			for _, l := range locs {
				if l == sec.label {
					dup = true
				}
			}
			if !dup {
				where[p] = append(locs, sec.label)
			}
		}
	}
	var out []Warning
	for _, p := range order {
		locs := where[p]
		shown := locs
		if len(shown) > 3 {
			shown = shown[:3]
		}
		more := ""
		if len(locs) > len(shown) {
			more = fmt.Sprintf(" (+%d more)", len(locs)-len(shown))
		}
		out = append(out, Warning(fmt.Sprintf(
			"Local path %s appears in %s%s; exported unchanged. Check it is fine to share.", p, strings.Join(shown, ", "), more)))
	}
	return out
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func nonNilExclusions(in []ExportExclusion) []ExportExclusion {
	if in == nil {
		return []ExportExclusion{}
	}
	return in
}

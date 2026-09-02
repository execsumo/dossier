package core

import (
	"fmt"
	"time"
)

// ArtifactType represents the classification of an archived source artifact.
type ArtifactType string

const (
	ArtifactTypeTranscript       ArtifactType = "transcript"
	ArtifactTypeSourceSnapshot   ArtifactType = "source_snapshot"
	ArtifactTypeFileSnapshot     ArtifactType = "file_snapshot"
	ArtifactTypeLink             ArtifactType = "link"
	ArtifactTypeQuery            ArtifactType = "query"
	ArtifactTypeDecisionEvidence ArtifactType = "decision_evidence"
)

// IsValid validates if the artifact type is one of the allowed enums.
func (t ArtifactType) IsValid() bool {
	switch t {
	case ArtifactTypeTranscript, ArtifactTypeSourceSnapshot, ArtifactTypeFileSnapshot,
		ArtifactTypeLink, ArtifactTypeQuery, ArtifactTypeDecisionEvidence:
		return true
	}
	return false
}

// ContentFormat represents the format of the artifact content.
type ContentFormat string

const (
	ContentFormatMarkdown ContentFormat = "markdown"
	ContentFormatJSON     ContentFormat = "json"
	ContentFormatText     ContentFormat = "txt"
)

// IsValid validates if the content format is supported.
func (f ContentFormat) IsValid() bool {
	switch f {
	case ContentFormatMarkdown, ContentFormatJSON, ContentFormatText:
		return true
	}
	return false
}

// Provenance represents the source location and context of the captured artifact.
type Provenance struct {
	Origin     string `yaml:"origin"`
	URL        string `yaml:"url,omitempty"`
	CapturedBy string `yaml:"captured_by,omitempty"`
	Harness    string `yaml:"harness,omitempty"`
}

// Artifact represents a captured source or reference document stored under a Dossier.
type Artifact struct {
	ID              string        `yaml:"id"`
	DossierID       string        `yaml:"dossier_id"`
	Type            ArtifactType  `yaml:"type"`
	Title           string        `yaml:"title"`
	CapturedAt      time.Time     `yaml:"captured_at"`
	RefreshedAt     time.Time     `yaml:"refreshed_at"`
	Frozen          bool          `yaml:"frozen"`
	Provenance      Provenance    `yaml:"provenance"`
	ContentFormat   ContentFormat `yaml:"content_format"`
	SourceSizeBytes int64         `yaml:"source_size_bytes"`
	Lines           int           `yaml:"lines"`
	Content         string        `yaml:"-"` // Not marshaled into frontmatter metadata
}

const MaxArtifactSizeBytes = 1024 * 1024 * 1024 // 1 GB limit

// Validate checks artifact constraints.
func (a *Artifact) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("artifact id is required")
	}
	if a.DossierID == "" {
		return fmt.Errorf("dossier id is required")
	}
	if !a.Type.IsValid() {
		return fmt.Errorf("invalid artifact type: %q", a.Type)
	}
	if a.Title == "" {
		return fmt.Errorf("artifact title is required")
	}
	if a.CapturedAt.IsZero() {
		return fmt.Errorf("captured_at is required")
	}
	if !a.ContentFormat.IsValid() {
		return fmt.Errorf("invalid content format: %q", a.ContentFormat)
	}
	if a.SourceSizeBytes > MaxArtifactSizeBytes {
		return fmt.Errorf("artifact size %d exceeds 1 GB limit", a.SourceSizeBytes)
	}
	return nil
}

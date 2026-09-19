package core

import "time"

// Conflict represents a rejected concurrent edit or merge conflict preserved for human resolution.
type Conflict struct {
	ID                 string     `yaml:"id" json:"id"`
	DossierID          string     `yaml:"dossier_id" json:"dossier_id"`
	Kind               string     `yaml:"kind" json:"kind"`
	BaseRevision       string     `yaml:"base_revision" json:"base_revision"`
	AttemptedRevision  string     `yaml:"attempted_revision" json:"attempted_revision"`
	Session            string     `yaml:"session,omitempty" json:"session,omitempty"`
	TS                 time.Time  `yaml:"ts" json:"ts"`
	RejectedBody       string     `yaml:"-" json:"rejected_body,omitempty"`
	DiffAgainstCurrent string     `yaml:"-" json:"diff_against_current,omitempty"`
	ResolvedAt         *time.Time `yaml:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	ResolvedBy         string     `yaml:"resolved_by,omitempty" json:"resolved_by,omitempty"`
	Choice             string     `yaml:"choice,omitempty" json:"choice,omitempty"`
}

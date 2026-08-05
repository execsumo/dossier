package core

import (
	"fmt"
	"time"
)

// Status represents the lifecycle state of a Dossier.
type Status string

const (
	StatusActive   Status = "active"
	StatusWaiting  Status = "waiting"
	StatusBlocked  Status = "blocked"
	StatusResolved Status = "resolved"
	StatusArchived Status = "archived"
)

// IsValid validates if the status is one of the allowed enums.
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusWaiting, StatusBlocked, StatusResolved, StatusArchived:
		return true
	}
	return false
}

// Normalize coerces an invalid or missing status toward attention.
// Backward-compatibility rule: a value that is no longer recognized (a removed
// enum member) or that is empty (a field added after the file was written) maps
// to the highest-attention valid value, so a stale Dossier surfaces rather than
// silently sinking. It returns the coerced value and whether a change was made;
// it is idempotent on already-valid values.
func (s Status) Normalize() (Status, bool) {
	if s.IsValid() {
		return s, false
	}
	return StatusActive, true
}

// Importance represents the priority dimension of importance.
type Importance string

const (
	ImportanceHigh Importance = "high"
	ImportanceLow  Importance = "low"
)

// IsValid validates if importance is one of the allowed enums.
func (i Importance) IsValid() bool {
	switch i {
	case ImportanceHigh, ImportanceLow:
		return true
	}
	return false
}

// Normalize coerces an invalid or missing importance toward attention.
// See Status.Normalize for the backward-compatibility rule; the highest-attention
// value for importance is "high".
func (i Importance) Normalize() (Importance, bool) {
	if i.IsValid() {
		return i, false
	}
	return ImportanceHigh, true
}

// Urgency represents the priority dimension of urgency.
type Urgency string

const (
	UrgencyHigh Urgency = "high"
	UrgencyLow  Urgency = "low"
)

// IsValid validates if urgency is one of the allowed enums.
func (u Urgency) IsValid() bool {
	switch u {
	case UrgencyHigh, UrgencyLow:
		return true
	}
	return false
}

// Normalize coerces an invalid or missing urgency toward attention.
// See Status.Normalize for the backward-compatibility rule; the highest-attention
// value for urgency is "high".
func (u Urgency) Normalize() (Urgency, bool) {
	if u.IsValid() {
		return u, false
	}
	return UrgencyHigh, true
}

// Interface identifies the forum where a dossier should be discussed.
type Interface string

const (
	InterfacePricingWBR    Interface = "Pricing WBR"
	InterfaceOneOnOne      Interface = "1:1"
	InterfaceOLGStandup    Interface = "OLG Standup"
	InterfaceGrowthStandup Interface = "Growth Standup"
	InterfaceSteerco       Interface = "Steerco"
	InterfaceSolutioning   Interface = "Solutioning"
	InterfaceOpsRev        Interface = "OpsRev"
)

// Interfaces is the canonical enum order used by validation and selectors.
var Interfaces = []Interface{
	InterfacePricingWBR,
	InterfaceOneOnOne,
	InterfaceOLGStandup,
	InterfaceGrowthStandup,
	InterfaceSteerco,
	InterfaceSolutioning,
	InterfaceOpsRev,
}

func (i Interface) IsValid() bool {
	for _, allowed := range Interfaces {
		if i == allowed {
			return true
		}
	}
	return false
}

// Frontmatter represents the parsed metadata block of a Dossier.
// In conformance with BUILD-DECISIONS, base_revision is session-side, not in frontmatter.
type Frontmatter struct {
	ID            string     `yaml:"id"`
	Name          string     `yaml:"name"`
	Slug          string     `yaml:"slug"`
	CreatedAt     time.Time  `yaml:"created_at"`
	UpdatedAt     time.Time  `yaml:"updated_at"`
	LastTouchedAt time.Time  `yaml:"last_touched_at"`
	Status        Status     `yaml:"status"`
	Lead          string     `yaml:"lead,omitempty"`
	Interfaces    []string   `yaml:"interfaces,omitempty"`
	NextAction    string     `yaml:"next_action"`
	OpenQuestions []string   `yaml:"open_questions"`
	Importance    Importance `yaml:"importance"`
	Urgency       Urgency    `yaml:"urgency"`
	DueDate       string     `yaml:"due_date,omitempty"`
	TokenTarget   int        `yaml:"token_target,omitempty"`
}

// FrontmatterFix records a single backward-compatibility coercion applied by
// Normalize, so the change can be surfaced to the user rather than done silently.
type FrontmatterFix struct {
	Field string
	From  string
	To    string
}

// Normalize brings frontmatter into conformance with the current schema for
// backward compatibility, mutating the receiver and returning the list of
// coercions made (empty if already canonical). It is the single, extensible
// place that heals Dossiers written by older builds:
//
//   - a value that is no longer a valid enum member (e.g. a removed "medium")
//     is mapped toward attention by the field's own Normalize;
//   - a field added after the file was written is empty, and likewise resolves
//     to its attention default.
//
// To support a new enum field, add its Normalize and one block here. The method
// is pure (no I/O) and idempotent.
func (f *Frontmatter) Normalize() []FrontmatterFix {
	var fixes []FrontmatterFix
	if v, changed := f.Status.Normalize(); changed {
		fixes = append(fixes, FrontmatterFix{Field: "status", From: string(f.Status), To: string(v)})
		f.Status = v
	}
	if v, changed := f.Importance.Normalize(); changed {
		fixes = append(fixes, FrontmatterFix{Field: "importance", From: string(f.Importance), To: string(v)})
		f.Importance = v
	}
	if v, changed := f.Urgency.Normalize(); changed {
		fixes = append(fixes, FrontmatterFix{Field: "urgency", From: string(f.Urgency), To: string(v)})
		f.Urgency = v
	}
	return fixes
}

// Validate ensures that all required fields are present and valid.
func (f *Frontmatter) Validate() error {
	if f.ID == "" {
		return fmt.Errorf("id is required")
	}
	if f.Name == "" {
		return fmt.Errorf("name is required")
	}
	if f.Slug == "" {
		return fmt.Errorf("slug is required")
	}
	if f.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if f.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required")
	}
	if f.LastTouchedAt.IsZero() {
		return fmt.Errorf("last_touched_at is required")
	}
	if !f.Status.IsValid() {
		return fmt.Errorf("invalid status: %q", f.Status)
	}
	if !f.Importance.IsValid() {
		return fmt.Errorf("invalid importance: %q", f.Importance)
	}
	if !f.Urgency.IsValid() {
		return fmt.Errorf("invalid urgency: %q", f.Urgency)
	}
	for _, interfaceName := range f.Interfaces {
		if !Interface(interfaceName).IsValid() {
			return fmt.Errorf("invalid interface: %q", interfaceName)
		}
	}
	return nil
}

// DistilledState contains the curated markdown representation of the topic.
type DistilledState struct {
	Body string
}

// Dossier represents the combined domain entity.
type Dossier struct {
	Frontmatter    Frontmatter
	DistilledState DistilledState
}

package core

import (
	"fmt"
	"time"
	"unicode/utf8"
)

// Status represents the lifecycle state of a Dossier.
type Status string

const (
	StatusSpark     Status = "spark"
	StatusDefine    Status = "define"
	StatusDelegated Status = "delegated"
	StatusReview    Status = "review"
	StatusBlocked   Status = "blocked"
	StatusDone      Status = "done"

	// Legacy statuses maintained for backward compatibility.
	StatusActive   Status = "active"
	StatusWaiting  Status = "waiting"
	StatusResolved Status = "resolved"
	StatusArchived Status = "archived"
)

// NormalizeStatus translates legacy status values into their canonical equivalents.
func NormalizeStatus(s Status) Status {
	switch s {
	case StatusActive:
		return StatusDefine
	case StatusWaiting:
		return StatusDelegated
	case StatusResolved, StatusArchived:
		return StatusDone
	default:
		return s
	}
}

// CanonicalStatuses returns the six canonical statuses in lifecycle order.
func CanonicalStatuses() []Status {
	return []Status{
		StatusSpark,
		StatusDefine,
		StatusDelegated,
		StatusReview,
		StatusBlocked,
		StatusDone,
	}
}

// IsValid validates if the status is one of the allowed enums (canonical or legacy).
func (s Status) IsValid() bool {
	switch NormalizeStatus(s) {
	case StatusSpark, StatusDefine, StatusDelegated, StatusReview, StatusBlocked, StatusDone:
		return true
	}
	return false
}

// IsTerminal reports whether the status represents completed/closed work.
func (s Status) IsTerminal() bool {
	return NormalizeStatus(s) == StatusDone
}

// IsOpen reports whether the status represents open/active work.
func (s Status) IsOpen() bool {
	return s.IsValid() && !s.IsTerminal()
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

// DefaultDiscussionInterfaces returns the legacy interface vocabulary in its
// display order. Callers receive a copy so configuration remains service-owned.
func DefaultDiscussionInterfaces() []string {
	return []string{
		string(InterfacePricingWBR),
		string(InterfaceOneOnOne),
		string(InterfaceOLGStandup),
		string(InterfaceGrowthStandup),
		string(InterfaceSteerco),
		string(InterfaceSolutioning),
		string(InterfaceOpsRev),
	}
}

// MaxNextActionLength is the maximum number of Unicode characters allowed in
// the concise current action stored in frontmatter.
const MaxNextActionLength = 140

func validateNextActionLength(nextAction string) error {
	if utf8.RuneCountInString(nextAction) > MaxNextActionLength {
		return fmt.Errorf("next_action must be at most %d characters", MaxNextActionLength)
	}
	return nil
}

// Frontmatter represents the parsed metadata block of a Dossier.
// In conformance with BUILD-DECISIONS, base_revision is session-side, not in frontmatter.
type Frontmatter struct {
	ID          string    `yaml:"id" json:"id"`
	Name        string    `yaml:"name" json:"name"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	Slug        string    `yaml:"slug" json:"slug"`
	CreatedAt   time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at" json:"updated_at"`
	Status      Status    `yaml:"status" json:"status"`
	Lead        string    `yaml:"lead,omitempty" json:"lead,omitempty"`
	Interfaces  []string  `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	NextAction  string    `yaml:"next_action" json:"next_action"`
	Priority    Priority  `yaml:"priority" json:"priority"`
	DueDate     string    `yaml:"due_date,omitempty" json:"due_date,omitempty"`
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
	if !f.Status.IsValid() {
		return fmt.Errorf("invalid status: %q", f.Status)
	}
	f.Status = NormalizeStatus(f.Status)
	if !f.Priority.IsValid() {
		return fmt.Errorf("invalid priority: %q", f.Priority)
	}
	if err := validateNextActionLength(f.NextAction); err != nil {
		return err
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

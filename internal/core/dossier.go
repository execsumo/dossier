package core

import (
	"fmt"
	"time"
	"unicode/utf8"
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
	if !f.Priority.IsValid() {
		return fmt.Errorf("invalid priority: %q", f.Priority)
	}
	if err := validateNextActionLength(f.NextAction); err != nil {
		return err
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

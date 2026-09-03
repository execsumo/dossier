package core

import (
	"testing"
	"time"
)

func TestStatusEnum(t *testing.T) {
	for _, status := range CanonicalStatuses() {
		if !status.IsValid() {
			t.Errorf("canonical status %q rejected", status)
		}
		if status == StatusDone {
			if !status.IsTerminal() || status.IsOpen() {
				t.Errorf("status %q should be terminal and not open", status)
			}
		} else {
			if status.IsTerminal() || !status.IsOpen() {
				t.Errorf("status %q should be open and not terminal", status)
			}
		}
	}

	legacyCases := []struct {
		legacy    Status
		canonical Status
	}{
		{StatusActive, StatusDefine},
		{StatusWaiting, StatusDelegated},
		{StatusBlocked, StatusBlocked},
		{StatusResolved, StatusDone},
		{StatusArchived, StatusDone},
	}
	for _, tc := range legacyCases {
		if !tc.legacy.IsValid() {
			t.Errorf("legacy status %q rejected", tc.legacy)
		}
		if got := NormalizeStatus(tc.legacy); got != tc.canonical {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", tc.legacy, got, tc.canonical)
		}
	}

	if Status("stalled").IsValid() {
		t.Fatal("unknown status accepted")
	}
}

func TestPriorityEnum(t *testing.T) {
	for _, priority := range []Priority{PriorityLow, PriorityMedium, PriorityHigh, PriorityMax} {
		if !priority.IsValid() {
			t.Errorf("priority %q rejected", priority)
		}
	}
	if Priority("urgent").IsValid() {
		t.Fatal("unknown priority accepted")
	}
}

func TestDiscussionInterfacesAreConfigurable(t *testing.T) {
	defaults := DefaultDiscussionInterfaces()
	if len(defaults) != 7 {
		t.Fatalf("default interfaces = %v, want seven legacy values", defaults)
	}
	defaults[0] = "mutated"
	if DefaultDiscussionInterfaces()[0] == "mutated" {
		t.Fatal("default interface slices share mutable storage")
	}

	now := time.Now()
	fm := Frontmatter{
		ID: "dos_interfaces", Name: "Interfaces", Slug: "interfaces",
		CreatedAt: now, UpdatedAt: now,
		Status: StatusActive, Priority: PriorityHigh,
		Interfaces: []string{"Custom Weekly Sync"},
	}
	if err := fm.Validate(); err != nil {
		t.Fatalf("structural validation rejected a configurable interface: %v", err)
	}
}

func TestFrontmatterRequiresCanonicalFields(t *testing.T) {
	now := time.Now()
	fm := Frontmatter{
		ID: "dos_x", Name: "Example", Slug: "example",
		CreatedAt: now, UpdatedAt: now, Status: StatusActive,
	}
	if err := fm.Validate(); err == nil {
		t.Fatal("missing priority accepted")
	}

	fm.Priority = PriorityHigh
	if err := fm.Validate(); err != nil {
		t.Fatalf("canonical frontmatter rejected: %v", err)
	}
}

func TestFrontmatterRejectsInvalidCanonicalValues(t *testing.T) {
	now := time.Now()
	fm := Frontmatter{
		ID: "dos_x", Name: "Example", Slug: "example",
		CreatedAt: now, UpdatedAt: now, Status: StatusActive,
		Priority: Priority("urgent"),
	}
	if err := fm.Validate(); err == nil {
		t.Fatal("invalid priority accepted")
	}
}

package core

import (
	"testing"
	"time"
)

func TestStatusEnum(t *testing.T) {
	for _, status := range []Status{StatusActive, StatusWaiting, StatusBlocked, StatusResolved, StatusArchived} {
		if !status.IsValid() {
			t.Errorf("status %q rejected", status)
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

func TestInterfaceEnum(t *testing.T) {
	for _, allowed := range Interfaces {
		if !allowed.IsValid() {
			t.Errorf("canonical interface %q rejected", allowed)
		}
	}
	if Interface("Weekly sync").IsValid() {
		t.Fatal("unknown interface accepted")
	}

	now := time.Now()
	fm := Frontmatter{
		ID: "dos_interfaces", Name: "Interfaces", Slug: "interfaces",
		CreatedAt: now, UpdatedAt: now,
		Status: StatusActive, Priority: PriorityHigh,
		Interfaces: []string{"1:1", "Pricing WBR"},
	}
	if err := fm.Validate(); err != nil {
		t.Fatalf("valid interfaces rejected: %v", err)
	}
	fm.Interfaces = []string{"weekly sync"}
	if err := fm.Validate(); err == nil {
		t.Fatal("invalid interface accepted")
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

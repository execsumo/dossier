package core

import (
	"testing"
	"time"
)

func TestCalculateRevision(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	fm1 := Frontmatter{
		ID:         "dos_test",
		Name:       "Test Dossier",
		Slug:       "test-dossier",
		CreatedAt:  now,
		UpdatedAt:  now,
		Status:     StatusActive,
		Priority:   PriorityHigh,
		NextAction: "Next step",
	}

	body1 := "This is a body.\r\nWith some trailing whitespace    \nAnd CRLF endings."

	// Test identical content with different formatting
	body2 := "This is a body.\nWith some trailing whitespace\nAnd CRLF endings."

	rev1 := CalculateRevision(fm1, body1, nil)
	rev2 := CalculateRevision(fm1, body2, nil)

	if rev1 != rev2 {
		t.Errorf("Expected revision to be identical regardless of CRLF or trailing whitespace: %s vs %s", rev1, rev2)
	}

	// Test change in field values changes revision
	fm2 := fm1
	fm2.Name = "Test Dossier Changed"
	rev3 := CalculateRevision(fm2, body1, nil)
	if rev1 == rev3 {
		t.Errorf("Expected revision to change when fields change")
	}

	// Test change in artifacts changes revision
	art1 := Artifact{
		ID:      "art_1",
		Content: "artifact content",
	}
	rev4 := CalculateRevision(fm1, body1, []Artifact{art1})
	if rev1 == rev4 {
		t.Errorf("Expected revision to change when artifacts list changes")
	}
}

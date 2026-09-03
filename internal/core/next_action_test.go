package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFrontmatterNextActionLength(t *testing.T) {
	tests := []struct {
		name      string
		next      string
		wantError bool
	}{
		{name: "at limit", next: strings.Repeat("a", MaxNextActionLength)},
		{name: "over limit", next: strings.Repeat("a", MaxNextActionLength+1), wantError: true},
		{name: "unicode at limit", next: strings.Repeat("界", MaxNextActionLength)},
		{name: "unicode over limit", next: strings.Repeat("界", MaxNextActionLength+1), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := Frontmatter{
				ID:         "dos_test",
				Name:       "Test dossier",
				Slug:       "test-dossier",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				Status:     StatusActive,
				NextAction: tt.next,
				Priority:   PriorityHigh,
			}

			err := fm.Validate()
			if (err != nil) != tt.wantError {
				t.Fatalf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSaveRejectsOverlongNextAction(t *testing.T) {
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{DossierHome: "/tmp/dossier-test"}, nil)

	_, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{
		"name":        "Too-long action",
		"next_action": strings.Repeat("a", MaxNextActionLength+1),
	}})
	if err == nil {
		t.Fatal("Save() accepted an overlong next_action")
	}
	if !strings.Contains(err.Error(), "next_action must be at most 140 characters") {
		t.Fatalf("Save() error = %v, want next_action length error", err)
	}
}

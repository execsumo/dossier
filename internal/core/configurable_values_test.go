package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func configurableValuesService(store Store, cfg Config) *Service {
	return NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, cfg, nil)
}

func TestSaveValidatesConfiguredInterfacesAndLeads(t *testing.T) {
	cfg := Config{
		DossierHome: "/tmp/dossier-test",
		Interfaces:  []string{"Customer Review", "Planning"},
		Leads:       []string{"Alice", "Bob"},
	}

	t.Run("configured values succeed", func(t *testing.T) {
		store := newLocalFakeStore()
		svc := configurableValuesService(store, cfg)
		_, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{
			"name":       "Configured values",
			"interfaces": []string{"Planning"},
			"lead":       "Alice",
		}})
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	})

	t.Run("unconfigured values fail", func(t *testing.T) {
		for field, value := range map[string]any{
			"interfaces": []string{"Steerco"},
			"lead":       "Carol",
		} {
			store := newLocalFakeStore()
			svc := configurableValuesService(store, cfg)
			_, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{"name": "Invalid", field: value}})
			if err == nil || !strings.Contains(err.Error(), "config.yaml") {
				t.Errorf("Save(%s) error = %v, want configured-value error", field, err)
			}
		}
	})

	t.Run("empty lead list preserves free form", func(t *testing.T) {
		store := newLocalFakeStore()
		svc := configurableValuesService(store, Config{DossierHome: "/tmp/dossier-test"})
		_, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{"name": "Free form", "lead": "Anyone"}})
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	})

	t.Run("malformed interface list fails", func(t *testing.T) {
		store := newLocalFakeStore()
		svc := configurableValuesService(store, cfg)
		_, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{"name": "Malformed", "interfaces": []any{"Planning", 42}}})
		if err == nil || !strings.Contains(err.Error(), "list of strings") {
			t.Fatalf("Save() error = %v, want list-of-strings error", err)
		}
	})
}

func TestLegacyConfiguredValuesDoNotBlockUnrelatedSave(t *testing.T) {
	store := newLocalFakeStore()
	now := time.Now()
	store.dossiers["dos_legacy"] = &Dossier{Frontmatter: Frontmatter{
		ID: "dos_legacy", Name: "Legacy", Slug: "legacy", CreatedAt: now, UpdatedAt: now,
		Status: StatusActive, Priority: PriorityHigh, Lead: "Former Lead", Interfaces: []string{"Former Meeting"},
	}}
	store.revisions["dos_legacy"] = "rev_legacy"

	svc := configurableValuesService(store, Config{
		DossierHome: "/tmp/dossier-test",
		Interfaces:  []string{"Current Meeting"},
		Leads:       []string{"Current Lead"},
	})
	if _, err := svc.Save(context.Background(), SaveReq{
		ID: "dos_legacy", BaseRevision: "rev_legacy", FrontmatterUpdates: map[string]any{"status": string(StatusWaiting)},
	}); err != nil {
		t.Fatalf("unrelated Save() rejected legacy values: %v", err)
	}
}

func TestServiceConfiguredValuesReturnCopies(t *testing.T) {
	svc := configurableValuesService(newLocalFakeStore(), Config{Interfaces: []string{"Planning"}, Leads: []string{"Alice"}})
	interfaces := svc.Interfaces()
	leads := svc.Leads()
	interfaces[0] = "mutated"
	leads[0] = "mutated"
	if svc.Interfaces()[0] != "Planning" || svc.Leads()[0] != "Alice" {
		t.Fatal("service exposed mutable configuration slices")
	}
}

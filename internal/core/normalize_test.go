package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestImportanceNormalize(t *testing.T) {
	cases := []struct {
		name    string
		in      Importance
		want    Importance
		changed bool
	}{
		{"valid high stays", ImportanceHigh, ImportanceHigh, false},
		{"valid low stays", ImportanceLow, ImportanceLow, false},
		{"removed value maps up", Importance("medium"), ImportanceHigh, true},
		{"unknown value maps up", Importance("urgent"), ImportanceHigh, true},
		{"missing value maps up", Importance(""), ImportanceHigh, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := tc.in.Normalize()
			if got != tc.want || changed != tc.changed {
				t.Fatalf("Normalize(%q) = (%q, %v), want (%q, %v)", tc.in, got, changed, tc.want, tc.changed)
			}
			// Idempotent: normalizing the result is a no-op.
			if got2, changed2 := got.Normalize(); got2 != got || changed2 {
				t.Fatalf("not idempotent: %q -> (%q, %v)", got, got2, changed2)
			}
		})
	}
}

func TestUrgencyNormalize(t *testing.T) {
	if got, changed := Urgency("medium").Normalize(); got != UrgencyHigh || !changed {
		t.Fatalf("medium urgency: got (%q, %v), want (high, true)", got, changed)
	}
	if got, changed := UrgencyLow.Normalize(); got != UrgencyLow || changed {
		t.Fatalf("valid low urgency should be unchanged, got (%q, %v)", got, changed)
	}
}

func TestStatusNormalize(t *testing.T) {
	if got, changed := Status("stalled").Normalize(); got != StatusActive || !changed {
		t.Fatalf("unknown status: got (%q, %v), want (active, true)", got, changed)
	}
	if got, changed := Status("").Normalize(); got != StatusActive || !changed {
		t.Fatalf("missing status: got (%q, %v), want (active, true)", got, changed)
	}
	if got, changed := StatusResolved.Normalize(); got != StatusResolved || changed {
		t.Fatalf("valid status should be unchanged, got (%q, %v)", got, changed)
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
		CreatedAt: now, UpdatedAt: now, LastTouchedAt: now,
		Status: StatusActive, Importance: ImportanceHigh, Urgency: UrgencyLow,
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

func TestFrontmatterNormalizeReportsAndHeals(t *testing.T) {
	fm := Frontmatter{
		ID:         "dos_x",
		Name:       "Legacy",
		Slug:       "legacy",
		Status:     "active",
		Importance: "medium", // removed enum value
		Urgency:    "",       // field absent in older build
	}
	fixes := fm.Normalize()

	if fm.Importance != ImportanceHigh || fm.Urgency != UrgencyHigh {
		t.Fatalf("expected coercion toward attention, got importance=%q urgency=%q", fm.Importance, fm.Urgency)
	}
	if len(fixes) != 2 {
		t.Fatalf("expected 2 fixes, got %d: %+v", len(fixes), fixes)
	}
	byField := map[string]FrontmatterFix{}
	for _, f := range fixes {
		byField[f.Field] = f
	}
	if got := byField["importance"]; got.From != "medium" || got.To != "high" {
		t.Fatalf("importance fix = %+v, want medium -> high", got)
	}
	if got := byField["urgency"]; got.From != "" || got.To != "high" {
		t.Fatalf("urgency fix = %+v, want '' -> high", got)
	}

	// Idempotent: a second pass changes nothing.
	if fixes2 := fm.Normalize(); len(fixes2) != 0 {
		t.Fatalf("expected no fixes on second pass, got %+v", fixes2)
	}
}

func TestPriorityScoreMapsLegacyValueTowardAttention(t *testing.T) {
	now := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	fm := Frontmatter{Importance: "medium", Urgency: UrgencyHigh}
	// medium importance must score as high importance (1: Do), not low (3: Delegate).
	if got := CalculatePriorityScore(fm, now); got != 1 {
		t.Fatalf("legacy medium importance scored %d, want 1 (toward attention)", got)
	}
}

func TestSaveHealsLegacyFrontmatterAndWarns(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)}, Config{TokenTarget: 100})
	ctx := context.Background()

	// Seed a dossier carrying a legacy "medium" importance directly in the store,
	// simulating a file written by an older build.
	fakeStore.dossiers["dos_fake_id"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:         "dos_fake_id",
			Name:       "Legacy",
			Slug:       "fake-slug",
			Status:     StatusActive,
			Importance: "medium",
			Urgency:    UrgencyLow,
		},
		DistilledState: DistilledState{Body: "# Legacy"},
	}
	fakeStore.revisions["dos_fake_id"] = "rev_seed"

	res, err := svc.Save(ctx, SaveReq{
		ID:                 "dos_fake_id",
		FrontmatterUpdates: map[string]any{"next_action": "touch"},
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// The healed value is persisted...
	d, _, err := fakeStore.Read("dos_fake_id")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if d.Frontmatter.Importance != ImportanceHigh {
		t.Fatalf("expected importance healed to high, got %q", d.Frontmatter.Importance)
	}

	// ...and the coercion is surfaced, not silent.
	var warned bool
	for _, w := range res.Warnings {
		if strings.Contains(string(w), "importance") && strings.Contains(string(w), "medium") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a warning about the importance coercion, got %v", res.Warnings)
	}
}

func TestMigrateHealsStoreWideAndIsIdempotent(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)}, Config{TokenTarget: 100})
	ctx := context.Background()

	// One legacy dossier and one already-canonical dossier.
	fakeStore.dossiers["dos_legacy"] = &Dossier{
		Frontmatter:    Frontmatter{ID: "dos_legacy", Name: "Legacy", Slug: "legacy", Status: StatusActive, Importance: "medium", Urgency: UrgencyLow},
		DistilledState: DistilledState{Body: "# Legacy"},
	}
	fakeStore.revisions["dos_legacy"] = "rev_legacy"
	fakeStore.dossiers["dos_ok"] = &Dossier{
		Frontmatter:    Frontmatter{ID: "dos_ok", Name: "OK", Slug: "ok", Status: StatusActive, Importance: ImportanceLow, Urgency: UrgencyLow},
		DistilledState: DistilledState{Body: "# OK"},
	}
	fakeStore.revisions["dos_ok"] = "rev_ok"

	res, err := svc.Migrate(ctx)
	if err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	rep := res.Data.(MigrateReport)
	if rep.DossiersScanned != 2 || rep.DossiersHealed != 1 {
		t.Fatalf("expected scanned=2 healed=1, got %+v", rep)
	}
	if d, _, _ := fakeStore.Read("dos_legacy"); d.Frontmatter.Importance != ImportanceHigh {
		t.Fatalf("legacy dossier not healed, importance=%q", d.Frontmatter.Importance)
	}

	// Idempotent: a second sweep heals nothing.
	res2, err := svc.Migrate(ctx)
	if err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}
	if rep2 := res2.Data.(MigrateReport); rep2.DossiersHealed != 0 {
		t.Fatalf("expected no heals on second sweep, got %+v", rep2)
	}
}

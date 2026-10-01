package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestListIncludesParsedRoutingLinksOnlyWhenRequested(t *testing.T) {
	fake := newLocalFakeStore()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fake.dossiers["dos_routes"] = &Dossier{
		Frontmatter:    Frontmatter{ID: "dos_routes", Name: "Routes", Slug: "routes", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now},
		DistilledState: DistilledState{Body: "## References\n- [ticket: OPS-7](https://example.test/7) — Ticket.\n\n## Active Monitors\n- [comms: #ops](https://example.test/ops) — Watch it. (Last polled: 2026-09-30)\n"},
	}
	svc := NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{DossierHome: "/tmp/dossier"}, nil)
	plain, err := svc.List(context.Background(), ListReq{})
	if err != nil {
		t.Fatal(err)
	}
	item := plain.Data.([]ListItem)[0]
	if len(item.Monitors) != 0 || len(item.References) != 0 {
		t.Fatalf("default list loaded links: %+v", item)
	}
	included, err := svc.List(context.Background(), ListReq{Include: []string{"monitors", "references"}})
	if err != nil {
		t.Fatal(err)
	}
	item = included.Data.([]ListItem)[0]
	if len(item.Monitors) != 1 || item.Monitors[0].LastPolled != "2026-09-30" || len(item.References) != 1 || item.References[0].Label != "OPS-7" {
		t.Fatalf("included routing links = %+v", item)
	}
}

func TestMonitorPolledUpdatesMonitorUsingSave(t *testing.T) {
	fake := newLocalFakeStore()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fake.dossiers["dos_routes"] = &Dossier{
		Frontmatter:    Frontmatter{ID: "dos_routes", Name: "Routes", Slug: "routes", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now},
		DistilledState: DistilledState{Body: "# Routes\n\n## Active Monitors\n- [comms: #ops](https://example.test/ops) — Watch it. (Last polled: 2026-09-30)\n"},
	}
	svc := NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
	if _, err := svc.MonitorPolled(context.Background(), MonitorPolledReq{ID: "dos_routes", URL: "https://example.test/ops"}); err != nil {
		t.Fatal(err)
	}
	if got := fake.dossiers["dos_routes"].DistilledState.Body; !strings.Contains(got, "(Last polled: 2026-10-01)") {
		t.Fatalf("monitor line not updated: %q", got)
	}
	if _, err := svc.MonitorPolled(context.Background(), MonitorPolledReq{ID: "dos_routes", URL: "https://missing.test"}); err == nil {
		t.Fatal("unknown monitor URL accepted")
	}
}

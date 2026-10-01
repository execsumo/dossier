package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAttentionAgentManagedAndFilterable(t *testing.T) {
	fake := newLocalFakeStore()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fake.dossiers["dos_attention"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_attention", Name: "Review", Slug: "review", Status: StatusExecute, Priority: PriorityHigh, CreatedAt: now, UpdatedAt: now}}
	svc := NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "alice"}, nil)
	updates := map[string]any{"attention": Attention{Level: "decide", Summary: "Approve scope"}}
	if _, err := svc.Save(context.Background(), SaveReq{ID: "dos_attention", Actor: "human:alice", FrontmatterUpdates: updates}); err == nil || !strings.Contains(err.Error(), "managed by agent or system") {
		t.Fatalf("human attention update error = %v", err)
	}
	if _, err := svc.Save(context.Background(), SaveReq{ID: "dos_attention", Actor: "agent:case-officer", FrontmatterUpdates: updates}); err != nil {
		t.Fatal(err)
	}
	attention := fake.dossiers["dos_attention"].Frontmatter.Attention
	if attention == nil || attention.Level != "decide" || attention.By != "agent:case-officer" || !attention.Since.Equal(now) {
		t.Fatalf("stored attention = %+v", attention)
	}
	filtered, err := svc.List(context.Background(), ListReq{Attention: "decide"})
	if err != nil || len(filtered.Data.([]ListItem)) != 1 || filtered.Data.([]ListItem)[0].Attention.Level != "decide" {
		t.Fatalf("attention filter = %+v, err=%v", filtered, err)
	}
	if _, err := svc.Save(context.Background(), SaveReq{ID: "dos_attention", Actor: "agent:case-officer", FrontmatterUpdates: map[string]any{"attention": nil}}); err != nil {
		t.Fatal(err)
	}
	if fake.dossiers["dos_attention"].Frontmatter.Attention != nil {
		t.Fatal("null attention did not clear the field")
	}
}

func TestHighImpactMutationRequiresHumanActor(t *testing.T) {
	svc := NewService(newLocalFakeStore(), &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, nil)
	if _, err := svc.Rename(context.Background(), RenameReq{Actor: "agent:case-officer", ID: "dos_missing", NewName: "Renamed"}); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent rename error = %v", err)
	}
	if _, err := svc.Merge(context.Background(), MergeReq{Actor: "agent:case-officer", SourceID: "dos_a", TargetID: "dos_b"}); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent merge error = %v", err)
	}
	if _, err := svc.Archive(context.Background(), ArchiveReq{Actor: "agent:case-officer", ID: "dos_missing"}); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent archive error = %v", err)
	}
}

func TestOnlyHumanActorMayMarkDone(t *testing.T) {
	fake := newLocalFakeStore()
	fake.dossiers["dos_done"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_done", Name: "Finish", Slug: "finish", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	svc := NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, nil)
	if _, err := svc.Save(context.Background(), SaveReq{ID: "dos_done", Actor: "agent:case-officer", FrontmatterUpdates: map[string]any{"status": "done"}}); err == nil {
		t.Fatal("agent marked dossier done")
	}
	if _, err := svc.Save(context.Background(), SaveReq{ID: "dos_done", Actor: "human:alice", FrontmatterUpdates: map[string]any{"status": "done"}}); err != nil {
		t.Fatalf("human could not mark done: %v", err)
	}
}
